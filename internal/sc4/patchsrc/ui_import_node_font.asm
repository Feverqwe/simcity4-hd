.intel_syntax noprefix
.text
# EAX is the new node, ESI is its cIGZWinTreeView.
cmp eax, -1
je done
pushad
mov ebx, eax
mov eax, 0x913c72
call eax
mov ecx, eax
mov edx, [ecx]
push 0xe9ba6423             # HDgenbodylight
call dword ptr [edx + 0x14] # GetFont
test eax, eax
jz restore
push eax
push ebx
mov ecx, esi
mov edx, [ecx]
call dword ptr [edx + 0x9c] # SetNodeFont(node, font)
restore:
popad
done:
ret
