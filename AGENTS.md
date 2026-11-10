# SimCity 4 HD agent guide

- Read `README.md` for the public interface. Read `_local/agent-memory/INDEX.md` if it exists, then open only notes relevant to the task.
- Treat local notes as dated evidence. Check the worktree, installed profile, and test results before relying on them. Keep public documentation independent of the local stand and machine paths.
- Distinguish screen geometry ×2, texture source density ×4, and projected atlases used by controls that draw directly at screen density. Trace the specific control and state before changing a shared atlas or render path.
- Verify UI fixes against original resources and the actual rebuilt package. Keep offline geometry and raster checks separate from game verification. Start the game only after separate user authorization.
- Keep extracted game assets, dumps, captures, machine-specific instructions, and diagnostic output in ignored local storage. Preserve unrelated changes. Commit only on request.
