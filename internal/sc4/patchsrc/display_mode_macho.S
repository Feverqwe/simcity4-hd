.intel_syntax noprefix
.text
.Lbase:
.macro branch op, target
    .byte \op
    .long \target - (0x1f00 + (. - .Lbase) + 4)
.endm

// Compare the graphics system's pre-init windowed flag with zero.
.macro test_windowed
    .byte 0x80, 0x3d
    .long 0xb6ed40 - (0x1f00 + (. - .Lbase) + 5)
    .byte 0
.endm

// Ensure the selected startup mode exists in the graphics driver's list.
    call .Lread
    test rax, rax
    jz .Ldone_modes
    call .LfindMode
    jc .Ldone_modes
    lea rdi, [rbp-0xe0]
    mov [rdi+0xc], eax
    shr rax, 32
    mov [rdi+8], eax
    mov dword ptr [rdi+0x5c], 32
    mov rsi, r14
    test_windowed
    jne .LaddWindowMode
    branch 0xe8, 0x60cf32
    jmp .Ldone_modes
.LaddWindowMode:
    branch 0xe8, 0x60cc86
.Ldone_modes:
    mov eax, [r14+0x228]
    branch 0xe9, 0x60cf0a

.org 0x60, 0x90
// Apply the selected mode to the preferences consumed by the startup pass.
    push rdi
    call .Lread
    pop rdi
    test rax, rax
    jz .LapplyPreferences
    mov rcx, [rdi+0x460]
    mov [rcx+0xf1c], rax
    mov dword ptr [rcx+0xf24], 32
    test_windowed
    sete byte ptr [rcx+0xf17] // Fullscreen flag in the game preferences.
    // The stock preference pass validates windowed mode against these metrics.
    mov [rip + .Lbase + 0xb117b8 - 0x1f00], rax
    mov dword ptr [rip + .Lbase + 0xb117c0 - 0x1f00], 32
.LapplyPreferences:
    mov esi, 1
    branch 0xe9, 0x80b2 // cSC4App::DoPreferencesChanged(true)

.Lread:
    sub rsp, 24
    lea rdi, [rip + .Lvar]
    branch 0xe8, 0x92b0f6
    test rax, rax
    jz .Ldefault
    cmp byte ptr [rax], 0
    jz .Ldefault
    mov rdi, rax
    lea rsi, [rip + .Lformat]
    lea rdx, [rsp]
    lea rcx, [rsp+4]
    lea r8, [rsp+8]
    xor eax, eax
    branch 0xe8, 0x92b612
    cmp eax, 2
    jne .Lbad
.Lvalidate:
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
.Ldefault:
    branch 0xe8, 0xb9b000 // Read saved UHD display preferences.
    test rax, rax
    jnz .Ldone_read
    test_windowed
    jne .Lbad
    branch 0xe8, 0x63f93c // MacDisplay::GetDisplayID
    test eax, eax
    jnz .LcopyMode
    branch 0xe8, 0x92aae4 // CGMainDisplayID
.LcopyMode:
    mov edi, eax
    branch 0xe8, 0x92aa30 // CGDisplayCopyDisplayMode
    test rax, rax
    jz .Lbad
    mov [rsp+8], rax
    mov rdi, rax
    branch 0xe8, 0x92aa66 // CGDisplayModeGetPixelWidth
    mov [rsp], eax
    mov rdi, [rsp+8]
    branch 0xe8, 0x92aa60 // CGDisplayModeGetPixelHeight
    mov [rsp+4], eax
    mov rdi, [rsp+8]
    branch 0xe8, 0x92aa78 // CGDisplayModeRelease
    jmp .Lvalidate
.Lbad:
    xor eax, eax
.Ldone_read:
    add rsp, 24
    ret

// CountVideoModes uses 56-byte records for both fullscreen and windowed
// modes. A matching record for the other mode does not satisfy this lookup.
.LfindMode:
    mov rdx, [r14+0x138]
    mov rsi, [r14+0x140]
    mov rcx, rax
    shr rcx, 32
    // mov r8b, [rip + windowed flag]
    .byte 0x44, 0x8a, 0x05
    .long 0xb6ed40 - (0x1f00 + (. - .Lbase) + 4)
    xor r8b, 1
.Lnext:
    cmp rdx, rsi
    jae .Lmissing
    cmp byte ptr [rdx+0x20], r8b
    jne .Ladvance
    cmp [rdx+4], eax
    jne .Ladvance
    cmp [rdx+8], ecx
    jne .Ladvance
    cmp dword ptr [rdx+12], 32
    je .Lfound
.Ladvance:
    add rdx, 56
    jmp .Lnext
.Lmissing:
    clc
    ret
.Lfound:
    stc
    ret
.Lvar: .asciz "SC4_UHD_RESOLUTION"
.Lformat: .asciz "%ux%u%c"
