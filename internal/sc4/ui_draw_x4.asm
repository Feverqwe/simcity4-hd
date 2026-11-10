# Steam 1.1.641.0 density adapters. Go relocates the external jumps.
.intel_syntax noprefix
.text
.global parser, direct, sliced, port, stretch, clipped

# The parser dispatch passes a writable copy of the complete atlas rectangle.
# Only replacement groups emitted for ×4 UI layouts use logical button geometry.
parser:
    mov ecx, [ebp+0x68]
    push dword ptr [ebp+0x64]
    mov eax, [ecx]
    mov edx, [ebp+0x54]
    sub edx, 0xe74a40b1
    cmp edx, 0x11
    ja parser_stock
    mov edx, [esp+8]
    test edx, edx
    je parser_stock
    or byte ptr [ecx+0xdf], 0x80
    sar dword ptr [edx], 1
    sar dword ptr [edx+4], 1
    sar dword ptr [edx+8], 1
    sar dword ptr [edx+12], 1
parser_stock:
    call dword ptr [eax+0x30]
    .byte 0xe9
    .long 0

# Original tail consumes the three already-pushed arguments.
direct:
    mov edx, [eax]
    test byte ptr [ecx+0xdf], 0x80
    mov ecx, eax
    jnz stretch_hd
    jmp dword ptr [edx+0x98]

# Mark the destination rectangle argument of this invocation without changing the helper's stack.
sliced:
    mov eax, [ecx+0x70]
    test byte ptr [ecx+0xdf], 0x80
    jz sliced_stock
    or byte ptr [esp+12], 1
sliced_stock:
    pop edx
    push eax
    push edx
    .byte 0xe9
    .long 0

port:
    mov ebx, [esp+0x4c]
    and ebx, -2
    mov ecx, [esp+0x2c]
    .byte 0xe9
    .long 0

stretch:
    test byte ptr [esp+0x5c], 1
    jnz stretch_hd
    mov eax, [ecx]
    jmp dword ptr [eax+0x98]
stretch_hd:
    mov edx, [esp+8]
    sub esp, 16
    mov eax, [edx]
    add eax, eax
    mov [esp], eax
    mov eax, [edx+4]
    add eax, eax
    mov [esp+4], eax
    mov eax, [edx+8]
    add eax, eax
    mov [esp+8], eax
    mov eax, [edx+12]
    add eax, eax
    mov [esp+12], eax
    mov edx, esp
    push dword ptr [esp+28]
    push edx
    push dword ptr [esp+28]
    mov eax, [ecx]
    call dword ptr [eax+0x98]
    add esp, 16
    ret 12

clipped:
    test byte ptr [esp+0x64], 1
    jnz clipped_hd
    mov eax, [ecx]
    jmp dword ptr [eax+0x9c]
clipped_hd:
    # Go relocates this jump to the native tile adapter.
    .byte 0xe9
    .long 0

.global bitmap_parser, bitmap_source, bitmap_direct, bitmap_sliced, stub_end
bitmap_parser:
    mov ecx, [ebp-4]
    mov eax, [ecx]
    mov edx, [ebp-8]
    sub edx, 0xe74a40b1
    cmp edx, 0x11
    ja bitmap_parser_stock
    or byte ptr [ecx+0x23], 0x80
bitmap_parser_stock:
    push edi
    call dword ptr [eax+0x10]
    .byte 0xe9
    .long 0
bitmap_source:
    test byte ptr [ebx+0xfb], 0x80
    jz bitmap_source_stock
    sar dword ptr [ebp-16], 1
    sar dword ptr [ebp-12], 1
    sar dword ptr [ebp-8], 1
    sar dword ptr [ebp-4], 1
bitmap_source_stock:
    lea esi, [ebx+0xd8]
    .byte 0xe9
    .long 0
bitmap_direct:
    test byte ptr [ebx+0xfb], 0x80
    jnz stretch_hd
    test byte ptr [ebx+0xfb], 0x40
    jnz bitmap_budget
    jmp dword ptr [eax+0x98]
bitmap_sliced:
    mov eax, [ebx+0x6c]
    test byte ptr [ebx+0xfb], 0x80
    jz sliced_stock
    or byte ptr [esp+12], 1
    jmp sliced_stock
bitmap_budget:
    # The budget bitmap keeps its stock source. Double the paint rectangle
    # from its existing origin to match the enlarged runtime control.
    mov edx, [esp+12]
    push eax
    mov eax, [edx+8]
    add eax, eax
    sub eax, [edx]
    mov [edx+8], eax
    mov eax, [edx+12]
    add eax, eax
    sub eax, [edx+4]
    mov [edx+12], eax
    pop eax
    jmp dword ptr [eax+0x98]
stub_end:
