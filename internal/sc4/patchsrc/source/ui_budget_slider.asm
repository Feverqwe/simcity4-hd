.intel_syntax noprefix
.text
# EAX is the newly created horizontal budget slider interface.
# Keep the fallback atlas if the matching DAT is absent.
mov esi, eax
sub esp, 0x30
mov ecx, esp
push 0
push 1
push 0x46a006a7
push 0xe74a40c6
push 0x856ddbac
mov eax, 0x602b70
call eax
mov edx, [esp + 4]
test edx, edx
jz release_image
mov eax, [esi]
push 0
push edx
mov ecx, esi
call dword ptr [eax + 0x30]  # SetImage, using the full four-frame atlas
release_image:
mov ecx, esp
mov eax, 0x602be0
call eax
add esp, 0x30
mov edx, [esi]
mov ecx, esi
