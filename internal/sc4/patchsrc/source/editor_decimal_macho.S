// Aspyr 185949, System V ABI. Assemble for x86_64-linux-gnu and extract .text.
.intel_syntax noprefix
.text
.Lbase:
.macro branch op, target
    .byte \op
    .long \target - (0xb9d000 + (. - .Lbase) + 4)
.endm

    push rdi
    push rsi
    sub rsp, 8
    call .Lnormalize
    add rsp, 8
    pop rsi
    pop rdi
    branch 0xe9, 0x92b68a // strtod preserves the end pointer.

.org 0x40, 0x90
    push rdi
    call .Lnormalize
    pop rdi
    branch 0xe9, 0x019b34 // RZAtofA, used by the curve editor.

// The editors pass local, writable text. Use the active CRT decimal
// character and keep every byte position intact for end-pointer validation.
.Lnormalize:
    push rbx
    mov rbx, rdi
    mov rdi, -2 // RTLD_DEFAULT on Darwin
    lea rsi, [rip + .LlocaleconvName]
    branch 0xe8, 0x92b036 // dlsym
    test rax, rax
    jz .LdefaultDecimal
    call rax
    test rax, rax
    jz .LdefaultDecimal
    mov rax, [rax] // struct lconv starts with decimal_point.
    test rax, rax
    jz .LdefaultDecimal
    cmp byte ptr [rax+1], 0
    jne .Ldone
    mov dl, [rax]
    test dl, dl
    jnz .Lscan
.LdefaultDecimal:
    mov dl, '.'
.Lscan:
    mov al, [rbx]
    test al, al
    jz .Ldone
    cmp al, '.'
    je .Lreplace
    cmp al, ','
    jne .Lnext
.Lreplace:
    mov [rbx], dl
.Lnext:
    inc rbx
    jmp .Lscan
.Ldone:
    pop rbx
    ret

.org 0xc0, 0x90
// Ignore edit notifications whose control ID isn't one of the 12 tax rates.
    lea ecx, [rax-0x100]
    cmp ecx, 11
    jbe .LtaxControl
    branch 0xe9, 0x2e4ba7
.LtaxControl:
    mov r15d, eax
    mov rdi, [r14+0x20]
    branch 0xe9, 0x2e4dd9

.org 0x100, 0x90
// Empty, partial and non-finite values keep the current spinner value.
// The valid branch applies the new value and then calls the same formatter.
    mov eax, eax
    test eax, eax
    jz .LinvalidTax
    add rbx, rax
    cmp rbx, [rbp-0x60]
    jne .LinvalidTax
    mov rax, [rbp-0x58]
    shr rax, 52
    and eax, 0x7ff
    cmp eax, 0x7ff
    je .LinvalidTax
    branch 0xe9, 0x2e5123
.LinvalidTax:
    branch 0xe9, 0x2e514c
.LlocaleconvName: .asciz "localeconv"
