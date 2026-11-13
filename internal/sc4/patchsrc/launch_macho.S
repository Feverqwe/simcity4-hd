// Aspyr 185949, System V ABI. These hooks occupy __TEXT padding at 0x1a80.
.intel_syntax noprefix
.text
.Lbase:
.macro branch op, target
    .byte \op
    .long \target - (0x1a80 + (. - .Lbase) + 4)
.endm

// Replace the call to the command-line constructor. The original string
// still supplies the game's language and preferences for direct launches.
    push rdi
    push rsi
    sub rsp, 8
    lea rdi, [rip + .Larguments]
    branch 0xe8, 0x92b0f6  // getenv
    test rax, rax
    jz .Loriginal
    cmp byte ptr [rax], 0
    je .Loriginal
    // Append to the string object and fetch its possibly reallocated data.
    lea rdi, [rbp-0x4c0]
    mov rsi, rax
    branch 0xe8, 0x92adc6  // std::string::append
    lea rcx, [rbp-0x4c0]
    lea rsi, [rcx+1]
    test byte ptr [rcx], 1
    cmovnz rsi, [rcx+16]
    mov [rsp+8], rsi
.Loriginal:
    add rsp, 8
    pop rsi
    pop rdi
    branch 0xe9, 0x14530

// Override the common profile resolver before it touches ~/Documents.
.org 0x80, 0x90
    push rdi
    push rsi
    sub rsp, 8
    lea rdi, [rip + .Lprofile]
    branch 0xe8, 0x92b0f6
    add rsp, 8
    pop rsi
    pop rdi
    test rax, rax
    jz .Ldocuments
    cmp byte ptr [rax], '/'
    jne .Ldocuments
    mov rsi, rax
    mov rax, [rdi]
    jmp [rax+0x28]
.Ldocuments:
    push rbp
    mov rbp, rsp
    push r15
    branch 0xe9, 0x39732

.Larguments:
    .asciz "SC4_UHD_ARGUMENTS"
.Lprofile:
    .asciz "SC4_UHD_PROFILE"

// PtInDisplay receives coordinates that SDLHandleEvent already converted
// to game pixels. Fullscreen bounds must use those same units.
.org 0x100, 0x90
    cmp byte ptr [rip + .Lbase + 0xb13130 - 0x1a80], 0
    jne .Lwindow_bounds
    mov eax, [rip + .Lbase + 0xb8f4d4 - 0x1a80]
    mov [rsi], eax
    mov eax, [rip + .Lbase + 0xb8f4d8 - 0x1a80]
    mov [rdx], eax
    ret
.Lwindow_bounds:
    branch 0xe9, 0x8d66b0
