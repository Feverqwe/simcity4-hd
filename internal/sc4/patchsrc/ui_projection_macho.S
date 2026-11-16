// Aspyr 185949. Assemble for x86_64-linux-gnu and extract .text.
.intel_syntax noprefix
.text

// RDI points to Aspyr's orthographic matrix. RSI and RDX are packed
// (left, top) and (right, bottom) from cGDriver's current viewport.
    mov eax, edx
    sub eax, esi
    jle .Ldone
    shr rdx, 32
    shr rsi, 32
    sub edx, esi
    jle .Ldone
    cvtsi2ss xmm0, eax
    cvtsi2ss xmm1, edx
    mov eax, 0x40000000 // 2.0
    movd xmm2, eax
    divss xmm2, xmm0
    movss [rdi], xmm2
    mov eax, 0xc0000000 // -2.0
    movd xmm2, eax
    divss xmm2, xmm1
    movss [rdi + 0x14], xmm2
.Ldone:
    ret
