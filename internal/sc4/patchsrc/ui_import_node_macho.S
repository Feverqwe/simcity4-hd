// Aspyr 185949, System V ABI. Assemble for x86_64-linux-gnu and extract .text.
.intel_syntax noprefix
.text

// Forward the five InsertNode arguments, then set the new row's font.
    sub rsp, 24
    mov [rsp], rdi
    mov [rsp + 8], r9
    call [rax + 0xc0] // InsertNode
    mov [rsp + 16], eax
    cmp eax, -1
    je .Ldone
    call [rsp + 8] // RZ::WCM()
    test rax, rax
    jz .Ldone
    mov rdi, rax
    mov rax, [rdi]
    mov esi, 0xe9ba6423 // HDgenbodylight
    call [rax + 0x28] // GetFont
    test rax, rax
    jz .Ldone
    mov rdi, [rsp]
    mov esi, [rsp + 16]
    mov rdx, rax
    mov rax, [rdi]
    call [rax + 0x138] // SetNodeFont
.Ldone:
    mov eax, [rsp + 16]
    add rsp, 24
    ret
