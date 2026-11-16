// Aspyr 185949. Assemble for x86_64-linux-gnu and extract .text.
.intel_syntax noprefix
.text

// Inline in cGZWinCombo::layout. RBX is the combo, ESI is its width.
    cmp qword ptr [rbx + 0x1b0], 0 // Bitmap arrow.
    jne .LtextWidth
    mov edx, [rbx + 0x1c]         // Window ID.
    cmp edx, 0xaa243f5e           // My Sims zodiac.
    je .Lresize
    cmp edx, 0xe74a4401           // Graphics resolution.
    je .Lresize
    cmp edx, 0xe74a4404           // Graphics window mode.
    jne .LtextWidth
.Lresize:
    movsx ecx, byte ptr [rbx + 0x1d8] // Button gutter.
    mov edx, esi
    sub edx, ecx
    xor ecx, ecx
    test edx, edx
    cmovl edx, ecx
    mov ecx, 32
    cmp edx, ecx
    cmovg edx, ecx
    mov [rbx + 0x1c0], edx
.LtextWidth:
    sub esi, [rbx + 0x1c0]
    ret

.org 0x60, 0x90
// The native combo uses a modal cGZWinListBox without a wheel override.
// The wheel can be delivered to the list or its scrollbar.
    lea r9, [rip + .Ltargets]
    movsxd rcx, dword ptr [r9]
    add rcx, r9 // cGZWinListBox vtable.
    mov rax, [rdi]
    cmp rax, rcx
    je .Lpopup
    movsxd rdx, dword ptr [r9 + 4]
    add rdx, r9
    cmp rax, rdx
    je .Lparent
    movsxd rdx, dword ptr [r9 + 8]
    add rdx, r9
    cmp rax, rdx
    jne .Lunhandled
.Lparent:
    mov rdi, [rdi + 0x58]
    test rdi, rdi
    jz .Lunhandled
    cmp [rdi], rcx
    jne .Lunhandled
.Lpopup:
    cmp byte ptr [rdi + 0x1a3], 0 // Popup list.
    je .Lunhandled
    mov eax, r8d
    cdq
    mov esi, 100 // The canvas delivers a wheel step as 100 units.
    idiv esi
    test eax, eax
    jz .Lunhandled
    mov esi, [rdi + 0x194]
    sub esi, eax
    push rdi
    push rsi
    sub rsp, 8
    mov rax, [rdi]
    call qword ptr [rax + 0x5a8] // GetLineCount.
    add rsp, 8
    pop rsi
    pop rdi
    cmp eax, [rdi + 0x190] // Visible line count.
    jle .Lunhandled
    lea r9, [rip + .Ltargets]
    movsxd rax, dword ptr [r9 + 12]
    add rax, r9 // SetIndexOfTopLine clamps, syncs the thumb and redraws.
    sub rsp, 8
    call rax
    add rsp, 8
    mov eax, 1
    ret
.Lunhandled:
    xor eax, eax
    ret
.Ltargets:
