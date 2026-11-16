.intel_syntax noprefix
.text
# The stored translation must satisfy dest = source * scale + translation.
fstp dword ptr [esp+8]
add eax, edi
mov [esp+0x34], eax
fild dword ptr [esp+0x34]
mov [esp+0x34], edi
fild dword ptr [esp+0x34]
fmul dword ptr [esp+12]
fsubp st(1), st(0)
fstp dword ptr [esp+4]
add ecx, ebx
mov [esp+0x34], ecx
fild dword ptr [esp+0x34]
mov [esp+0x34], ebx
fild dword ptr [esp+0x34]
fmul dword ptr [esp+8]
fsubp st(1), st(0)
fstp dword ptr [esp]
mov eax, [esp+0x30]
mov ecx, esi
