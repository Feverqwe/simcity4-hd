"""Load the local HAT checkpoint and upscale RGBA images on CUDA."""
import argparse
import hashlib
from io import BytesIO
import struct
import sys
from pathlib import Path

import numpy as np
from PIL import Image

WEIGHTS = Path('4x_IllustrationJaNai_V3detail_HAT_L_28k_bf16.safetensors')
SHA256 = '38c00c9ff871ae292617ae1127edc150f81c4e0f8f98685e48ff9c9cd87fba3b'

def verify_checkpoint(weights):
    with Path(weights).open('rb') as source:
        if hashlib.file_digest(source, 'sha256').hexdigest() != SHA256:
            raise ValueError(f'unexpected HAT checkpoint checksum for {weights}')


def load_upscale(weights=WEIGHTS, *, scale=2, tile=64, overlap=16):
    import torch
    from spandrel import ImageModelDescriptor, ModelLoader

    if tile <= 0 or overlap < 0:
        raise ValueError("expected a positive tile size and nonnegative overlap")
    if scale not in (2, 4):
        raise ValueError('expected output scale 2 or 4')
    verify_checkpoint(weights)
    if not torch.cuda.is_available():
        raise RuntimeError('CUDA is unavailable in the WSL environment')
    model = ModelLoader().load_from_file(weights)
    if not isinstance(model, ImageModelDescriptor) or model.scale != 4:
        raise ValueError('expected a 4x image upscaling model')
    model = model.eval().cuda().bfloat16()
    print(torch.__version__, torch.cuda.get_device_name(), file=sys.stderr, flush=True)

    @torch.inference_mode()
    def upscale(frame):
        width, height = frame.size
        result = Image.new('RGB', (width * scale, height * scale))
        rgb = frame.convert('RGB')
        for y in range(0, height, tile):
            for x in range(0, width, tile):
                right, bottom = min(width, x + tile), min(height, y + tile)
                left, top = max(0, x - overlap), max(0, y - overlap)
                crop = rgb.crop((left, top, min(width, right + overlap), min(height, bottom + overlap)))
                tensor = torch.from_numpy(np.array(crop)).cuda().permute(2, 0, 1)[None]
                tensor = tensor.to(torch.bfloat16).div_(255)
                output = model(tensor)[0].clamp(0, 1).mul(255).round().byte().permute(1, 2, 0)
                image = Image.fromarray(output.cpu().numpy())
                image = image.resize((crop.width * scale, crop.height * scale), Image.Resampling.LANCZOS)
                box = tuple(value * scale for value in (x - left, y - top, right - left, bottom - top))
                result.paste(image.crop(box), (x * scale, y * scale))
        result.putalpha(frame.getchannel('A').resize(result.size, Image.Resampling.LANCZOS))
        return result

    return upscale


MAX_IMAGE_BYTES = 64 << 20


def serve(upscale, source, destination):
    while header := source.read(4):
        if len(header) != 4:
            raise ValueError("truncated request length")
        length, = struct.unpack("<I", header)
        if not 0 < length <= MAX_IMAGE_BYTES:
            raise ValueError("invalid request size")
        payload = source.read(length)
        if len(payload) != length:
            raise ValueError("truncated request image")
        with Image.open(BytesIO(payload), formats=("PNG",)) as image:
            result = upscale(image.convert("RGBA"))
        with BytesIO() as encoded:
            result.save(encoded, format="PNG")
            response = encoded.getvalue()
        if len(response) > MAX_IMAGE_BYTES:
            raise ValueError("response exceeds the image size limit")
        destination.write(struct.pack("<I", len(response)))
        destination.write(response)
        destination.flush()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--weights", type=Path, required=True)
    parser.add_argument("--scale", type=int, choices=(2, 4), required=True)
    parser.add_argument("--tile", type=int, required=True)
    parser.add_argument("--overlap", type=int, required=True)
    args = parser.parse_args()
    upscale = load_upscale(args.weights, scale=args.scale, tile=args.tile, overlap=args.overlap)
    serve(upscale, sys.stdin.buffer, sys.stdout.buffer)


if __name__ == "__main__":
    main()
