// Aspyr 185949, System V ABI. Assemble for x86_64-linux-gnu and extract .text.
.intel_syntax noprefix
.text

// rdi = size vector, esi/edx = viewport, rcx = stock vector growth.
    push rbx
    push r12
    push r13
    push r14
    push r15
    sub rsp, 16
    mov r12, rdi
    mov r13d, esi
    mov r14d, edx
    mov r15, rcx
    mov eax, 1
    test r13d, r13d
    cmovle r13d, eax
    test r14d, r14d
    cmovle r14d, eax

    lea rbx, [rip + .Lsizes]
.Lnext:
    mov eax, [rbx]
    test eax, eax
    jz .Lfull
    cmp eax, r13d
    jg .Lfull
    cmp [rbx + 4], r14d
    jg .Lskip
    mov rax, [rbx]
    mov [rsp], rax
    mov rsi, rsp
    call .Lappend
.Lskip:
    add rbx, 8
    jmp .Lnext

.Lfull:
    mov [rsp], r13d
    mov [rsp + 4], r14d
    mov rax, [r12 + 8]
    cmp rax, [r12]
    je .LappendFull
    mov rdx, [rsp]
    cmp [rax - 8], rdx
    je .Ldone
.LappendFull:
    mov rsi, rsp
    call .Lappend
.Ldone:
    add rsp, 16
    pop r15
    pop r14
    pop r13
    pop r12
    pop rbx
    ret

.Lappend:
    mov rax, [r12 + 8]
    cmp rax, [r12 + 16]
    je .Lgrow
    mov rdx, [rsi]
    mov [rax], rdx
    add rax, 8
    mov [r12 + 8], rax
    ret
.Lgrow:
    mov rdi, r12
    jmp r15

.org 0xc0, 0x90
    paddd xmm0, [rip + .LtextInset]
    ret
.p2align 4
.LtextInset:
    .long 8, 8, -8, -8

.Lsizes:
    .long 320, 240
    .long 640, 480
    .long 960, 720
    .long 1280, 960
    .long 1600, 1200
    .long 2048, 1536
    .long 2560, 2048
    .long 3200, 2400
    .long 3600, 2700
    .long 4096, 3072
    .long 0, 0
