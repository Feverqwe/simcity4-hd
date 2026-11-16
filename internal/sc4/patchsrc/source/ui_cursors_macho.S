// Aspyr 185949, System V ABI. Assemble for x86_64-linux-gnu and extract .text.
// The cursor hooks start at the optional executable segment's +0xd00.
.intel_syntax noprefix
.text
.Lbase:
.macro branch op, target
    .byte \op
    .long \target - (0xb9ad00 + (. - .Lbase) + 4)
.endm

// Preserve the stock directory checks for 32x32. The enlarged CUR entries
// use 64x64 or 128x128 and have no palette.
    movzx eax, byte ptr [rbp-0x431]
    cmp eax, 32
    jne .Llarge
    branch 0xe9, 0x577cd
.Llarge:
    cmp eax, 64
    je .LsizeAccepted
    cmp eax, 128
    jne .LrejectDirectory
.LsizeAccepted:
    cmp al, [rbp-0x432]
    jne .LrejectDirectory
    cmp word ptr [rbp-0x440], 2
    jne .LrejectDirectory
    cmp byte ptr [rbp-0x439], 0
    jne .LrejectDirectory
    branch 0xe9, 0x577fc
.LrejectDirectory:
    branch 0xe9, 0x57abe

// The stock loader assumes a 32-pixel AND mask and can treat the first
// BGRA word as a palette entry. Read checked, bottom-up BGRA rows directly
// into the game's buffer for an enlarged cursor. The alpha channel in the
// replacement CUR already accounts for its mask.
.org 0x80, 0x90
    test al, al
    jz .LrejectBitmap
    cmp dword ptr [rbp-0x568], 108
    je .LdecodeAlpha
    cmp byte ptr [rbp-0x431], 32
    jne .LrejectBitmap
    mov cl, [rbp-0x55a]
    branch 0xe9, 0x57975
.LdecodeAlpha:
    cmp word ptr [rbp-0x440], 2
    jne .LrejectBitmap
    movzx r12d, byte ptr [rbp-0x431]
    cmp [rbp-0x564], r12d
    jne .LrejectBitmap
    movzx r13d, byte ptr [rbp-0x432]
    lea eax, [r13+r13]
    cmp [rbp-0x560], eax
    jne .LrejectBitmap
    cmp dword ptr [rbp-0x55c], 0x00200001
    jne .LrejectBitmap
    cmp dword ptr [rbp-0x558], 3
    jne .LrejectBitmap
    cmp dword ptr [rbp-0x548], 0
    jne .LrejectBitmap
    cmp [rbp-0x43e], r12w
    jae .LrejectBitmap
    cmp [rbp-0x43c], r13w
    jae .LrejectBitmap
    cmp r12d, r13d
    jne .LrejectBitmap

    // The legacy palette scratch is available for the remaining V4 header.
    lea rdi, [rbp-0x540]
    lea rsi, [rbp-0x430]
    mov edx, 68
    branch 0xe8, 0x36658
    test al, al
    jz .LrejectBitmap
    cmp dword ptr [rbp-0x430], 0x00ff0000
    jne .LrejectBitmap
    cmp dword ptr [rbp-0x42c], 0x0000ff00
    jne .LrejectBitmap
    cmp dword ptr [rbp-0x428], 0x000000ff
    jne .LrejectBitmap
    cmp dword ptr [rbp-0x424], 0xff000000
    jne .LrejectBitmap

    shl r12d, 2
    mov rdi, [rbp-0x448]
    mov rdi, [rdi]
    mov rax, [rdi]
    call [rax+0x120]
    test rax, rax
    jz .LrejectBitmap
    mov r15, rax
    mov rdi, [rbp-0x448]
    mov rdi, [rdi]
    mov rax, [rdi]
    call [rax+0x128]
    cmp eax, r12d
    jl .LrejectBitmap
    movsxd rbx, eax
    lea eax, [r13-1]
    imul rax, rbx
    add r15, rax
    neg rbx
.Lrow:
    lea rdi, [rbp-0x540]
    mov rsi, r15
    mov edx, r12d
    branch 0xe8, 0x36658 // cRZFile::Read
    test al, al
    jz .LrejectBitmap
    add r15, rbx
    dec r13d
    jnz .Lrow
    branch 0xe9, 0x57d8f // Build the stock CGImage and register the cursor.
.LrejectBitmap:
    mov rbx, [rbp-0x448]
    branch 0xe9, 0x57aa9 // Release the rejected buffer.

// Stock cursors have a magenta color key. Enlarged cursors preserve
// every BGRA sample, including opaque magenta and partial coverage.
.org 0x220, 0x90
    cmp dword ptr [rbp-0x568], 108
    je .LuseAlpha
    cmp eax, [rbp-0x450]
    cmovne ecx, eax
    branch 0xe9, 0x57e35
.LuseAlpha:
    mov ecx, eax
    branch 0xe9, 0x57e35

// CUR hotspots start in bitmap pixels. AppKit uses points for both the
// image size and hotspot, and uses the original pixels for the Retina rep.
.org 0x240, 0x90
    push r15
    xor r15d, r15d
    cmp dword ptr [rbp-0x568], 108
    jne .LregisterCursor
    movzx r15d, byte ptr [rbp-0x431]
.LregisterCursor:
    branch 0xe8, 0x7f9d76 // MacAddNamedCursor
    pop r15
    ret

.org 0x260, 0x90
    movsd xmm0, [rbp-0x38]
    movsd xmm1, [rbp-0x40]
    test r15d, r15d
    jz .LpointHotspot
    mulsd xmm0, [rip + .LretinaScale]
    mulsd xmm1, [rip + .LretinaScale]
.LpointHotspot:
    ret

.org 0x280, 0x90
    sub rsp, 8
    call r14 // NSImage initWithCGImage:size: uses NSZeroSize.
    mov rbx, rax
    test r15d, r15d
    jz .LimageDone
    test rax, rax
    jz .LstockSize
    // Setting the image's point size after initialization retains the
    // original pixel representation. Passing size to init resamples it.
    mov rdi, -2 // RTLD_DEFAULT
    lea rsi, [rip + .LselectorLookup]
    branch 0xe8, 0x92b036 // dlsym
    test rax, rax
    jz .LstockSize
    lea rdi, [rip + .LsetSize]
    call rax
    mov rsi, rax
    mov rdi, rbx
    cvtsi2sd xmm0, r15d
    mulsd xmm0, [rip + .LretinaScale]
    movapd xmm1, xmm0
    call r14
    jmp .LimageDone
.LstockSize:
    xor r15d, r15d
.LimageDone:
    add rsp, 8
    ret
.org 0x2d9, 0x90
.LretinaScale:
    .double 0.5 // The builder selects the matching density for this representation.
.LselectorLookup:
    .asciz "sel_registerName"
.LsetSize:
    .asciz "setSize:"
