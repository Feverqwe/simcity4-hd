// Aspyr 185949. Assemble for x86_64-linux-gnu and extract .text.
.intel_syntax noprefix
.text

// These are inline hooks in cSC4WinMapView::Update. RAX points to the
// bitmap bounds before the row loop. Inside the loop it is scratch, and
// the frame holds the inclusive right edge and the left edge separately.
.macro stroke entry, operation, output, stock, hd, inside_loop=0
    .org \entry, 0x90
    pushfq
    push rcx
    .if \inside_loop
        mov ecx, [rbp - 0x11c]
        sub ecx, [rbp - 0xc0]
        cmp ecx, 511
    .else
        mov ecx, [rax + 8]
        sub ecx, [rax]
        cmp ecx, 512
    .endif
    pop rcx
    jge .Lhd\@
    \operation \output, dword ptr [rip + .Lstock\@]
    popfq
    ret
.Lhd\@:
    \operation \output, dword ptr [rip + .Lscaled\@]
    popfq
    ret
.Lstock\@:
    .float \stock
.Lscaled\@:
    .float \hd
.endm

stroke 0x00, movss, xmm1, -0.5, -1.0
stroke 0x40, movss, xmm1, 0.5, 1.0
stroke 0x80, addss, xmm0, 1.0, 2.0
stroke 0xc0, movss, xmm1, 2.0, 4.0, 1
stroke 0x100, mulss, xmm0, 128.0, 64.0, 1
