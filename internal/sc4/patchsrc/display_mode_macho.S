.intel_syntax noprefix
.text
.Lbase:
.macro branch op, target
    .byte \op
    .long \target - (0x1f00 + (. - .Lbase) + 4)
.endm

// At the end of CountVideoModes, append the explicitly requested
// mode through the game's enumerator, which handles both mode lists.
    call .Lread
    test rax, rax
    jz .Ldone_modes
    lea rdi, [rbp-0xe0]
    mov [rdi+0xc], eax
    shr rax, 32
    mov [rdi+8], eax
    mov dword ptr [rdi+0x5c], 32
    mov rsi, r14
    branch 0xe8, 0x60cf32
.Ldone_modes:
    mov eax, [r14+0x228]
    branch 0xe9, 0x60cf0a

.org 0x60, 0x90
// Seed the desired startup mode with the same width and height.
    push rax
    push rdi
    push rcx
    sub rsp, 8
    call .Lread
    movabs rdx, 0x30000000400
    test rax, rax
    cmovnz rdx, rax
    add rsp, 8
    pop rcx
    pop rdi
    pop rax
    branch 0xe9, 0x546b

.Lread:
    sub rsp, 24
    lea rdi, [rip + .Lvar]
    branch 0xe8, 0x92b0f6
    test rax, rax
    jz .Ldone_read
    mov rdi, rax
    lea rsi, [rip + .Lformat]
    lea rdx, [rsp]
    lea rcx, [rsp+4]
    xor eax, eax
    branch 0xe8, 0x92b612
    cmp eax, 2
    jne .Lbad
    cmp dword ptr [rsp], 800
    jb .Lbad
    cmp dword ptr [rsp], 7680
    ja .Lbad
    cmp dword ptr [rsp+4], 600
    jb .Lbad
    cmp dword ptr [rsp+4], 4320
    ja .Lbad
    mov rax, [rsp]
    jmp .Ldone_read
.Lbad:
    xor eax, eax
.Ldone_read:
    add rsp, 24
    ret
.Lvar: .asciz "SC4_UHD_RESOLUTION"
.Lformat: .asciz "%ux%u"
