// Aspyr 185949, System V ABI. Assemble for x86_64-linux-gnu and extract .text.
.intel_syntax noprefix
.text
.Lbase:
.macro branch op, target
    .byte \op
    .long \target - (0xb9e400 + (. - .Lbase) + 4)
.endm

// Unpack the caller's resource key without mutating it. Both constructors
// initialize the same cSLBuffer fields and load the same resource type.
    mov r9d, ecx
    mov r8d, edx
    mov ecx, [rsi+8]
    mov edx, [rsi+4]
    mov esi, [rsi]
    jmp .Lparts

.org 0x20, 0x90
.Lparts:
    sub rsp, 40
    mov [rsp], rdi
    mov [rsp+8], esi
    mov [rsp+12], edx
    mov [rsp+16], ecx
    mov [rsp+20], r8d
    mov [rsp+24], r9d
    mov edx, 0xe74a40cc // Projected runtime artwork from the UHD UI package.
    branch 0xe8, 0x043c72
    mov rdi, [rsp]
    cmp qword ptr [rdi+8], 0
    jne .Ldone

    // A failed constructor still initializes the temporary resource object.
    branch 0xe8, 0x043df0
    mov rdi, [rsp]
    mov esi, [rsp+8]
    mov edx, [rsp+12]
    mov ecx, [rsp+16]
    mov r8d, [rsp+20]
    mov r9d, [rsp+24]
    add rsp, 40
    branch 0xe9, 0x043c72
.Ldone:
    add rsp, 40
    ret
