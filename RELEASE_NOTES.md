Share chats and memories between profiles.

- The wizard's new **Share chats and memories between profiles** option lets each profile use the shared chats, the shared memories, both, or neither. `multi-codex-app share ID all|chats|memories|off` does the same from the command line.
- Shared chats are live links to one copy in the owner profile (profile 1 unless you choose another with `share owner ID`), so every sharing app sees the same chats without copying or syncing.
- Turning sharing off gives a profile back its own chats and memories exactly as they were. Nothing is deleted, and chats started while sharing stay readable in the shared history.
- `multi-codex-app doctor` checks sharing links and explains how to repair them.
- Sharing is available on macOS and Linux.

Credit to Edi Hasaj for the original independent-profile methodology.
