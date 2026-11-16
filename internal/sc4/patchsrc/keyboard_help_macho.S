// Aspyr 185949, System V ABI. The Play Options hooks use __TEXT padding at 0x1c00.
.intel_syntax noprefix
.text
.Lbase:
.macro branch op, target
    .byte \op
    .long \target - (0x1c00 + (. - .Lbase) + 4)
.endm

// Connect the two new buttons to the same root as Credits.
    push rbx
    push r12
    mov rdi, [r14]
    mov rax, [rdi]
    call [rax+0x18]
    mov rbx, rax
    mov rdi, rbx
    mov esi, 0xe74a4302
    mov rax, [rdi]
    call [rax+0x118]
    test rax, rax
    jz .LinitDone
    mov r12, rax
    mov rdi, rbx
    mov esi, 0xe74a4301
    call .Lconnect
    mov rdi, r12
    mov esi, 0xe74a4304
    call .Lconnect
.LinitDone:
    pop r12
    pop rbx
    mov rdi, [r14]
    mov rax, [rdi]
    branch 0xe9, 0x3a32be
.Lconnect:
    sub rsp, 8
    mov rax, [rdi]
    call [rax+0x118]
    test rax, rax
    jz .Lconnected
    mov rdi, rax
    mov rsi, rbx
    mov rax, [rdi]
    call [rax+0x2b0]
.Lconnected:
    add rsp, 8
    ret

// The stock message listener has confirmed the source root is visible.
.org 0x80, 0x90
    mov rdi, rbx
    mov rsi, r15
    call .Lroute
    test al, al
    jz .Lstock
    mov r14d, 1
    branch 0xe9, 0x3a354f
.Lstock:
    cmp dword ptr [r15], 3
    je .LstockButton
    branch 0xe9, 0x3a354c
.LstockButton:
    branch 0xe9, 0x3a351e

.Lroute:
    push rbx
    push r12
    push r13
    push r14
    push r15
    sub rsp, 64
    mov rbx, rdi
    mov r15, rsi
    xor r13d, r13d
    cmp dword ptr [r15], 3
    jne .Lkey
    cmp qword ptr [r15+8], 0x287259f6
    jne .Lunhandled
    mov r13d, [r15+16]
    jmp .Lpanel
.Lkey:
    cmp dword ptr [r15], 4
    jne .Lunhandled
    cmp byte ptr [r15+8], 27
    jne .Lunhandled
.Lpanel:
    mov rdi, rbx
    mov esi, 0xe74a4302
    mov rax, [rdi]
    call [rax+0x118]
    test rax, rax
    jz .Lunhandled
    mov r14, rax
    mov rdi, r14
    mov rax, [rdi]
    call [rax+0x238]
    test al, al
    jnz .Lvisible
    cmp r13d, 0xe74a4301
    jne .Lunhandled
    jmp .Loptions
.Lvisible:
    cmp dword ptr [r15], 4
    je .Lclosing
    cmp r13d, 0xe74a4304
    jne .Lhandled
.Lclosing:
    xor r13d, r13d
.Loptions:
    mov rdi, rbx
    mov esi, 0x2a57db84
    mov rax, [rdi]
    call [rax+0x118]
    test rax, rax
    jz .Lhandled
    mov rbx, rax
    test r13d, r13d
    jz .Lswap

    mov rdi, rbx
    mov rax, [rdi]
    call [rax+0x238]
    test al, al
    jz .Lhandled
    mov qword ptr [rsp+56], 0
    mov rdi, r14
    mov esi, 0xe74a4303
    mov edx, 0x231a1c57
    lea rcx, [rsp+56]
    mov rax, [rdi]
    call [rax+0x128]
    test al, al
    jz .Lhandled
    mov r12, [rsp+56]
    test r12, r12
    jz .Lhandled

    mov rdi, rsp
    mov esi, 0xe74a40da
    mov edx, 0xe74a43e2
    xor ecx, ecx
    branch 0xe8, 0x121650  // cSLString::cSLString(group, instance, false)
    mov rdi, rsp
    branch 0xe8, 0x1715c   // cRZString::Strlen()
    mov r13d, eax
    test eax, eax
    jz .Lrelease
    mov rdi, r12
    mov rsi, rsp
    xor edx, edx
    branch 0xe8, 0x1fb514  // cIGZWinTextEdit thunk to SetText()
.Lrelease:
    mov rdi, rsp
    branch 0xe8, 0x121738  // cSLString::~cSLString()
    mov rdi, r12
    mov rax, [rdi]
    call [rax+0x10]
    test r13d, r13d
    jz .Lhandled
.Lswap:
    // rbx is the options window, r14 is help. Hide the currently visible one.
    test r13d, r13d
    jnz .Lset
    xchg rbx, r14
.Lset:
    mov rdi, rbx
    mov esi, 1
    xor edx, edx
    mov rax, [rdi]
    call [rax+0x220]
    mov rdi, r14
    mov esi, 1
    mov edx, 1
    mov rax, [rdi]
    call [rax+0x220]
.Lhandled:
    mov eax, 1
    jmp .Lreturn
.Lunhandled:
    xor eax, eax
.Lreturn:
    add rsp, 64
    pop r15
    pop r14
    pop r13
    pop r12
    pop rbx
    ret
