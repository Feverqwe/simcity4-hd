// Aspyr 185949, System V ABI. Assemble for x86_64-linux-gnu and extract .text.
// The executable segment reserves this page at RVA 0xb9c000.
.intel_syntax noprefix
.text
.Lbase:
.macro branch op, target
    .byte \op
    .long \target - (0xb9c000 + (. - .Lbase) + 4)
.endm

.set inWindowMode, 0xb13130
.set mainWindow, 0xb8f4e8
.set gameWidth, 0xb8f4d4
.set gameHeight, 0xb8f4d8
.set mouseScaleX, 0xb78078
.set mouseScaleY, 0xb7807c

// The fullscreen path already uses a fixed-size CGL surface. Apply the
// same override to the windowed context before exposing resize handles.
// r15 is the CGL context and r14 points to the original width and height.
    mov esi, 304
    mov rdi, r15
    mov rdx, r14
    branch 0xe8, 0x92aade // CGLSetParameter
    test eax, eax
    jz .Lenable
    cmp byte ptr [rip + .Lbase + inWindowMode - 0xb9c000], 0
    jne .Lresize
.Lenable:
    mov esi, 305
    mov rdi, r15
    branch 0xe8, 0x92aad2 // CGLEnable
.Lresize:
    cmp byte ptr [rip + .Lbase + inWindowMode - 0xb9c000], 0
    je .LsetupDone
    mov rdi, [rip + .Lbase + mainWindow - 0xb9c000]
    test rdi, rdi
    jz .LsetupDone
    test eax, eax
    sete al
    movzx esi, al
    branch 0xe8, 0x8a8fb0 // SDL_SetWindowResizable
.LsetupDone:
    branch 0xe9, 0x63fc98

.org 0x80, 0x90
// Recalculate the mouse transform before motion, button and wheel events.
// SDL window dimensions and event positions are both in logical points.
    mov eax, [r13]
    sub eax, 0x400
    cmp eax, 3
    ja .LinputDone
    call .LwindowScale
    test eax, eax
    jz .LnativeScale
    movss [rip + .Lbase + mouseScaleX - 0xb9c000], xmm0
    movss [rip + .Lbase + mouseScaleY - 0xb9c000], xmm1
    jmp .LinputDone
.LnativeScale:
    lea rdi, [rip + .Lbase + mouseScaleX - 0xb9c000]
    lea rsi, [rip + .Lbase + mouseScaleY - 0xb9c000]
    branch 0xe8, 0x63f914 // MacDisplay::GetDisplayScale
.LinputDone:
    branch 0xe9, 0x21fc

.org 0x100, 0x90
// GetCursorPosition polls SDL independently of the most recent mouse event.
// Map the polled position through the same window-to-render transform.
.LpollPosition:
    sub rsp, 24
    mov [rsp], rdi
    mov [rsp+8], rsi
    branch 0xe8, 0x8d6600 // SDL_GetMouseState
    call .LwindowScale
    test eax, eax
    jnz .LmapPolledPosition
.LcursorDone:
    add rsp, 24
    ret

// Map SDL points to render pixels in fullscreen and in resizable windows.
// A non-resizable window uses Aspyr's stock one-to-one windowed input path.
.LwindowScale:
    mov rdi, [rip + .Lbase + mainWindow - 0xb9c000]
    test rdi, rdi
    jz .Lunscaled
    sub rsp, 8
    cmp byte ptr [rip + .Lbase + inWindowMode - 0xb9c000], 0
    je .LmeasureWindow
    branch 0xe8, 0x8a89b0 // SDL_GetWindowFlags
    test al, 0x20 // SDL_WINDOW_RESIZABLE
    jz .LnoWindowSize
.LmeasureWindow:
    mov rdi, [rip + .Lbase + mainWindow - 0xb9c000]
    mov qword ptr [rsp], 0
    mov rsi, rsp
    lea rdx, [rsp+4]
    branch 0xe8, 0x8d66b0 // SDL_GetWindowSize
    mov eax, [rsp]
    mov edx, [rsp+4]
    add rsp, 8
    test eax, eax
    jle .Lunscaled
    test edx, edx
    jle .Lunscaled
    mov ecx, [rip + .Lbase + gameWidth - 0xb9c000]
    mov esi, [rip + .Lbase + gameHeight - 0xb9c000]
    test ecx, ecx
    jle .Lunscaled
    test esi, esi
    jle .Lunscaled
    mov r8d, eax
    mov r9d, edx
    cvtsi2ss xmm0, ecx
    cvtsi2ss xmm2, eax
    divss xmm0, xmm2
    cvtsi2ss xmm1, esi
    cvtsi2ss xmm2, edx
    divss xmm1, xmm2
    mov eax, 1
    ret
.LnoWindowSize:
    add rsp, 8
.Lunscaled:
    xor eax, eax
    ret

.org 0x1d0, 0x90
// Window-manager cursor moves are specified in render pixels. SDL wants
// window-relative logical coordinates after the window is resized.
    sub rsp, 24
    mov [rsp], edi
    mov [rsp+4], esi
    cmp byte ptr [rip + .Lbase + inWindowMode - 0xb9c000], 0
    je .LnativeWarp
    call .LwindowScale
    mov edi, [rsp]
    mov esi, [rsp+4]
    test eax, eax
    jz .LnativeWarp
    cvtsi2ss xmm2, esi
    divss xmm2, xmm1
    cvtss2si edx, xmm2
    cvtsi2ss xmm2, edi
    divss xmm2, xmm0
    cvtss2si esi, xmm2
    mov rdi, [rip + .Lbase + mainWindow - 0xb9c000]
    branch 0xe8, 0x828c40 // SDL_WarpMouseInWindow
    mov eax, 1
    add rsp, 24
    ret
.LnativeWarp:
    add rsp, 24
    branch 0xe9, 0x642414 // SetCursorPos

// Edge scrolling compares the polled position with the last render pixel.
// Ordinary scaling can leave that pixel unreachable when a point spans
// multiple pixels. Preserve the interior mapping and map the last point
// on each axis to the corresponding edge.
.LmapPolledPosition:
    mov r10d, esi
    mov rdi, [rsp]
    mov rsi, [rsp+8]
    dec r8d
    jz .LscaleX
    cmp [rdi], r8d
    jne .LscaleX
    lea eax, [rcx-1]
    jmp .LwriteX
.LscaleX:
    cvtsi2ss xmm2, dword ptr [rdi]
    mulss xmm2, xmm0
    cvttss2si eax, xmm2
.LwriteX:
    mov [rdi], eax
    dec r9d
    jz .LscaleY
    cmp [rsi], r9d
    jne .LscaleY
    lea eax, [r10-1]
    jmp .LwriteY
.LscaleY:
    cvtsi2ss xmm2, dword ptr [rsi]
    mulss xmm2, xmm1
    cvttss2si eax, xmm2
.LwriteY:
    mov [rsi], eax
    jmp .LcursorDone

.org 0x280, 0x90
// Region scrolling still asks the inherited graphics system whether it
// is fullscreen. Use Aspyr's current window mode and leave the result in
// ebx, as the displaced virtual call and mov did.
    cmp byte ptr [rip + .Lbase + inWindowMode - 0xb9c000], 0
    jne .LedgeInactive
    mov rdi, [rip + .Lbase + mainWindow - 0xb9c000]
    test rdi, rdi
    jz .LedgeInactive
    // SDL_GetMouseState can still report (0, 0) before the pointer enters.
    sub rsp, 8
    branch 0xe8, 0x8a89b0 // SDL_GetWindowFlags
    add rsp, 8
    and eax, 0x600 // SDL_WINDOW_INPUT_FOCUS | SDL_WINDOW_MOUSE_FOCUS
    xor ebx, ebx
    cmp eax, 0x600
    sete bl
    ret
.LedgeInactive:
    xor ebx, ebx
    ret

.org 0x2c0, 0x90
// The region covers the root window. Poll SDL instead of using the window
// manager's last dispatched mouse event, which can lag at the screen edge.
    mov rdi, rdx
    mov rsi, rcx
    jmp .LpollPosition

.org 0x2e0, 0x90
// Aspyr builds the Direct3D viewport from GetClientRect. A resizable
// window already has a fixed CGL backing, so use its render dimensions.
    cmp byte ptr [rip + .Lbase + inWindowMode - 0xb9c000], 0
    je .LrenderBounds
    mov rdi, [rip + .Lbase + mainWindow - 0xb9c000]
    test rdi, rdi
    jz .LclientBounds
    branch 0xe8, 0x8a89b0 // SDL_GetWindowFlags
    test al, 0x20 // SDL_WINDOW_RESIZABLE
    jz .LclientBounds
.LrenderBounds:
    branch 0xe9, 0x63fdb6
.LclientBounds:
    branch 0xe9, 0x63fdcf
