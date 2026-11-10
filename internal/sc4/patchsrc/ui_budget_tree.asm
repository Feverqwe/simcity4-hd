.intel_syntax noprefix
.text
scale_window:
    push ebp
    mov ebp, esp
    push ebx
    push esi
    push edi
    sub esp, 16
    mov ebx, [ebp + 8]
    mov ecx, ebx
    mov eax, [ecx]
    call dword ptr [eax + 0xc0]  # GetArea()
    mov esi, eax
    mov edi, esp
    movsd
    movsd
    movsd
    movsd

    # Ordinances also returns through this hook when it opens the
    # separately scaled empty message. Only scale the 450 px ledger.
    cmp dword ptr [ebp + 12], 2
    jne enum_window
    mov eax, [esp + 8]
    sub eax, [esp]
    cmp eax, 450
    jne window_done
enum_window:
    # Get a position-independent pointer to the enumeration callback.
    call callback_base
callback_base:
    pop eax
    add eax, offset callback_delta
    push dword ptr [ebp + 16]    # Pass the font manager to children.
    push eax
    push 0x22ba0121             # cIGZWin IID
    mov ecx, ebx
    mov eax, [ecx]
    call dword ptr [eax + 0x80]  # EnumChildren

    mov eax, [esp]
    mov edx, [esp + 4]
    mov esi, [esp + 8]
    mov edi, [esp + 12]
    sub esi, eax
    sub edi, edx
    cmp dword ptr [ebp + 12], 0
    je child_origin
    mov ecx, esi
    sar ecx, 1
    sub eax, ecx
    mov ecx, edi
    sar ecx, 1
    sub edx, ecx
    jmp set_area
child_origin:
    add eax, eax
    add edx, edx
set_area:
    lea edi, [edx + edi*2]
    lea esi, [eax + esi*2]
    push edi
    push esi
    push edx
    push eax
    mov ecx, ebx
    mov eax, [ecx]
    call dword ptr [eax + 0xdc]  # SetArea(left, top, right, bottom)
    mov edx, [ebp + 16]
    test edx, edx
    jz window_done
    mov ecx, ebx
    call scale_font
window_done:
    add esp, 16
    pop edi
    pop esi
    pop ebx
    pop ebp
    ret

# cIGZWin::EnumChildren callback. The child is borrowed for this call.
scale_child:
    push dword ptr [esp + 16]
    push 0
    push dword ptr [esp + 20]
    call scale_window
    add esp, 12
    mov eax, 1
    ret

.set callback_delta, scale_child - callback_base

# Apply the HD font after sizing. The constructors use SetFont directly,
# so a window may have a font pointer even when GetFontStyle has no value.
scale_font:
    push ebp
    push ebx
    push esi
    push edi
    sub esp, 8
    mov ebx, edx
    mov edi, ecx
    mov eax, [ecx]
    push esp
    push 0x212cdc1f             # cIGZWinText
    call dword ptr [eax]
    test al, al
    jz try_button
    mov esi, [esp]
    mov edi, [esi + 4]
    mov dword ptr [esp + 4], 0x48
    jmp find_font
try_button:
    mov ecx, edi
    mov eax, [ecx]
    push esp
    push 0x8810                 # cIGZWinBtn
    call dword ptr [eax]
    test al, al
    jz try_bitmap
    mov esi, [esp]
    mov edi, [esi + 0x108]
    mov dword ptr [esp + 4], 0x4c
find_font:
    test edi, edi
    jz release_control
    call font_table
font_table:
    pop ebp
    add ebp, offset font_delta
next_font:
    mov eax, [ebp]
    test eax, eax
    jz release_control
    push eax
    mov ecx, ebx
    mov eax, [ecx]
    call dword ptr [eax + 0x14]  # GetFont(style)
    cmp eax, edi
    je replace_font
    add ebp, 8
    jmp next_font
replace_font:
    push dword ptr [ebp + 4]
    mov ecx, ebx
    mov eax, [ecx]
    call dword ptr [eax + 0x14]
    test eax, eax
    jz release_control
    mov edx, [esp + 4]
    push eax
    mov ecx, esi
    mov eax, [ecx]
    call dword ptr [eax + edx]   # SetFont(font)
    jmp release_control
try_bitmap:
    mov ecx, edi
    mov eax, [ecx]
    push esp
    push 0xc12cea13             # cIGZWinBMP
    call dword ptr [eax]
    test al, al
    jz font_done
    mov esi, [esp]
    mov ecx, esi
    mov eax, [ecx]
    push 1
    push 0x40000000             # Mark runtime budget bitmaps for a doubled destination.
    call dword ptr [eax + 0x2c]
release_control:
    mov ecx, esi
    mov eax, [ecx]
    call dword ptr [eax + 8]
font_done:
    add esp, 8
    pop edi
    pop esi
    pop ebx
    pop ebp
    ret

font_styles:
    .long 0x4a809916, 0x346d92f4 # GenHeader
    .long 0x4a809919, 0x60199608 # GenButton
    .long 0xea85d304, 0xd1eeddae # BdgtLedgerHeader
    .long 0xea85d305, 0x1d1329ca # BdgtLedgerLineLite
    .long 0xac0c7170, 0x5a259567 # BdgtLedgerLineLiteUnder
    .long 0xea85d306, 0x442327e3 # BdgtLedgerLineHeavy
    .long 0
.set font_delta, font_styles - font_table
