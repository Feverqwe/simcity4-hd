// Aspyr 185949, System V ABI. Assemble for x86_64-linux-gnu and extract .text.
.intel_syntax noprefix
.text

// rdi = cIGZWin*, rsi = proposed cRZRect*. DisplayLegend also uses the
// updated caption rectangle to measure the text and advance the next row.
    push rdi
    push rsi
    sub rsp, 8
    mov rax, [rdi]
    call [rax + 0x178] // GetArea
    add rsp, 8
    pop rsi
    pop rdi
    mov ecx, [rax]
    mov [rsi], ecx
    mov ecx, [rax + 8]
    mov [rsi + 8], ecx
    mov rax, [rdi]
    jmp [rax + 0x1b8] // SetArea
