// Aspyr Steam 185949, x86-64 System V ABI. This code lives in __TEXT
// after the Mach-O load commands, at image offset 0x1600.
.intel_syntax noprefix
.text
.Lbase:
.macro jump_rva target
    .byte 0xe9
    .long \target - (0x1600 + (. - .Lbase) + 4)
.endm
.macro dense_group reg
    sub \reg, 0xe74a40b1
    cmp \reg, 0x11
.endm
.macro half_rect ptr
    movdqu xmm0, [\ptr]
    psrad xmm0, 1
    movdqu [\ptr], xmm0
.endm

// The button parser passes the whole atlas rectangle to SetStyle.
.org 0x00, 0x90
    mov r10d, [rbp-0x518]
    dense_group r10d
    seta r10b
    and byte ptr [rdi+0x15b], 0x7f
    test r10b, r10b
    jnz .Lbutton_set
    or byte ptr [rdi+0x15b], 0x80
    half_rect rcx
.Lbutton_set:
    mov rdx, rbx
    call [rax+0x60]
    jump_rva 0x53fb2a

// Mark the bitmap before SetBMP calculates its source limits.
.org 0x50, 0x90
    mov r10d, [rbp-0x38]
    dense_group r10d
    seta r10b
    and byte ptr [rdi+0x28], 0x7f
    test r10b, r10b
    jnz .Lbitmap_set
    or byte ptr [rdi+0x28], 0x80
.Lbitmap_set:
    mov rsi, rbx
    call [rax+0x20]
    jump_rva 0x540478

.org 0x90, 0x90
    mov esi, r15d
    mov edx, eax
    mov r10d, [rbp-0x38]
    dense_group r10d
    ja .Lbitmap_resize
    shr esi, 1
    shr edx, 1
.Lbitmap_resize:
    call r13
    jump_rva 0x5404ab

.org 0xc0, 0x90
    lea rsi, [rbp-0x70]
    test byte ptr [rdi+0x28], 0x80
    jz .Lbitmap_area
    half_rect rsi
.Lbitmap_area:
    call [rax+0x40]
    jump_rva 0x540523

// Bitmap paint takes either the stored logical crop or the full buffer.
.org 0xf0, 0x90
    test byte ptr [rbx+0x178], 0x80
    jz .Lbitmap_full
    lea rcx, [rbp-0x38]
    movdqu xmm0, [rax]
    psrad xmm0, 1
    movdqu [rcx], xmm0
    jump_rva 0x330860
.Lbitmap_full:
    mov rcx, [rax]
    mov rax, [rax+8]
    jump_rva 0x330858

.org 0x130, 0x90
    test byte ptr [rdi+0x15b], 0x80
    setnz r11b
    mov rax, [rdi+0xa8]
    mov rsi, [rdi+0x1f8]
    mov r8, [rax]
    lea rdx, [rbp-0x10]
    mov rdi, rax
    test r11b, r11b
    jnz .Lbutton_blt
    call [r8+0x130]
    jump_rva 0x28119f
.Lbutton_blt:
    call .Lstretch
    jump_rva 0x28119f

.org 0x180, 0x90
    mov r8d, 1
    test byte ptr [rdi+0x15b], 0x80
    jz .Lbutton_edge
    or r8d, 0x100
.Lbutton_edge:
    mov rdi, rax
    jump_rva 0x281207

.org 0x1b0, 0x90
    mov r8d, 1
    test byte ptr [rbx+0x148], 0x80
    jz .Lbitmap_edge
    or r8d, 0x100
.Lbitmap_edge:
    mov rcx, rbx
    jump_rva 0x3308ca

.org 0x1e0, 0x90
    lea rcx, [rbp-0x48]
    test byte ptr [rbx+0x178], 0x80
    jnz .Lbitmap_blt
    call [rax+0x130]
    jump_rva 0x33095a
.Lbitmap_blt:
    call .Lstretch
    jump_rva 0x33095a

// Sliced frames keep their original destination partitioning. The high
// argument bit stays in the helper's stack slot and is invisible to its
// byte-sized center-fill test.
.org 0x220, 0x90
    test byte ptr [rbp-0x5b], 1
    jnz .Lstretch
    jmp [rax+0x130]
.org 0x240, 0x90
    test byte ptr [rbp-0x5b], 1
    jnz .Ltile
    jmp [rax+0x138]

// cIGZDrawContext dispatch. Map the physical source origin to the
// destination origin, then draw with the existing clipping and alpha state.
.org 0x260, 0x90
.Lstretch:
    mov r9d, [rdx+8]
    sub r9d, [rdx]
    jle .Lempty
    mov r10d, [rdx+12]
    sub r10d, [rdx+4]
    jle .Lempty
    mov eax, [rcx+8]
    sub eax, [rcx]
    jle .Lempty
    cvtsi2ss xmm2, eax
    mov eax, [rcx+12]
    sub eax, [rcx+4]
    jle .Lempty
    cvtsi2ss xmm3, eax
    add r9d, r9d
    add r10d, r10d
    cvtsi2ss xmm4, r9d
    cvtsi2ss xmm5, r10d
    divss xmm2, xmm4
    divss xmm3, xmm5
    cvtsi2ss xmm0, dword ptr [rcx]
    cvtsi2ss xmm1, dword ptr [rcx+4]
    mov eax, [rdx]
    add eax, eax
    cvtsi2ss xmm4, eax
    mov eax, [rdx+4]
    add eax, eax
    cvtsi2ss xmm5, eax
    mulss xmm4, xmm2
    mulss xmm5, xmm3
    subss xmm0, xmm4
    subss xmm1, xmm5
    push rdi
    push rcx
    sub rsp, 8
    mov rax, [rdi]
    call [rax+0xe8]
    mov rdi, [rsp+16]
    mov rax, [rdi]
    call [rax+0xf0]
    add rsp, 8
    pop rsi
    pop rdi
    push rdi
    mov rax, [rdi]
    call [rax+0x118]
    pop rdi
    mov rax, [rdi]
    jmp [rax+0xe0]

.Lempty:
    ret

.org 0x340, 0x90
.Ltile:
    mov eax, [rdx+8]
    cmp eax, [rdx]
    jle .Lempty
    mov eax, [rdx+12]
    cmp eax, [rdx+4]
    jle .Lempty
    mov eax, [rcx+8]
    cmp eax, [rcx]
    jle .Lempty
    mov eax, [rcx+12]
    cmp eax, [rcx+4]
    jle .Lempty
    add r8d, [rcx]
    add r9d, [rcx+4]
    cvtsi2ss xmm0, r8d
    cvtsi2ss xmm1, r9d
    mov eax, 0x3f000000
    movd xmm2, eax
    movaps xmm3, xmm2
    push rdi
    push rcx
    sub rsp, 24
    movdqu xmm4, [rdx]
    pslld xmm4, 1
    movdqu [rsp], xmm4
    mov rax, [rdi]
    call [rax+0xe8]
    mov rdi, [rsp+32]
    mov rsi, rsp
    mov rax, [rdi]
    call [rax+0xf8]
    mov rdi, [rsp+32]
    mov rsi, [rsp+24]
    mov rax, [rdi]
    call [rax+0x118]
    mov rdi, [rsp+32]
    mov rax, [rdi]
    call [rax+0xf0]
    add rsp, 32
    pop rdi
    mov rax, [rdi]
    jmp [rax+0xe0]

// My Sims updates these bitmaps outside the layout parser.
.org 0x400, 0x90
    mov rdi, [rbp-0x48]
    call .Lset_preview
    jump_rva 0x316e40
.org 0x420, 0x90
    mov rdi, [rbp-0x18]
    call .Lset_preview
    jump_rva 0x31b1d8
.Lset_preview:
    or byte ptr [rdi+0x28], 0x80
    mov rax, [rdi]
    jmp [rax+0x20]

// SetBMP clips the window crop to the bitmap's logical bounds.
.org 0x440, 0x90
    sub rsp, 8
    mov rax, [rdi]
    call [rax+0x58]
    jmp .Llogical_limit
.org 0x450, 0x90
    sub rsp, 8
    mov rax, [rdi]
    call [rax+0x60]
.Llogical_limit:
    add rsp, 8
    test byte ptr [r14+0x178], 0x80
    jz .Llimit_return
    shr eax, 1
.Llimit_return:
    ret
