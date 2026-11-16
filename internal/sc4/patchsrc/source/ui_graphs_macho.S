// Aspyr 185949, System V ABI. Assemble for x86_64-linux-gnu and extract .text.
.intel_syntax noprefix
.text

// Keep the plot clear of the widened legend column.
    sub ecx, 212
    sub ecx, [rbp - 0x78]
    ret

.org 0x10, 0x90
// Anchor the bitmap button at the start of the legend.
    lea esi, [rcx - 204]
    sub ecx, 188
    ret

.org 0x20, 0x90
    sub r15d, 186
    sub r15d, ebx
    ret

.org 0x30, 0x90
    sub r15d, 202
    sub r15d, [rbp - 0x78]
    ret

.org 0x40, 0x90
// Draw a second scanline for the series only, clamped to the plot's top.
// rdi = graph, rsi = buffer, rdx = segment endpoints, ecx = series.
    sub rsp, 56
    movups xmm0, [rdx]
    movups [rsp], xmm0
    mov [rsp + 16], rdi
    mov [rsp + 24], rsi
    mov [rsp + 32], ecx
    mov rax, [rdi]
    call [rax + 0x7f8]

    mov rdi, [rsp + 16]
    mov eax, [rdi + 0x160] // cGZGraph plot top.
    cmp [rsp + 4], eax
    jle .Lsecond
    dec dword ptr [rsp + 4]
.Lsecond:
    cmp [rsp + 12], eax
    jle .Ldraw
    dec dword ptr [rsp + 12]
.Ldraw:
    mov rsi, [rsp + 24]
    mov rdx, rsp
    mov ecx, [rsp + 32]
    mov rax, [rdi]
    call [rax + 0x7f8]
    add rsp, 56
    ret
