.text
.intel_syntax noprefix
    # Each hooked editor passes a writable copy of its text.
    # Keep the byte count and original pointer for the tax end-pointer check.
    mov eax, [esp + 4]
    mov dl, byte ptr [0xb19c60]  # Decimal character used by the CRT.
.scan:
    mov cl, [eax]
    test cl, cl
    jz .done
    cmp cl, ','
    je .replace
    cmp cl, '.'
    jne .next
.replace:
    mov [eax], dl
.next:
    inc eax
    jmp .scan
.done:
    # The builder appends the jump to the stock parser.
