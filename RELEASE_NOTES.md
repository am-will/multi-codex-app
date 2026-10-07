Manage individual profiles from the wizard without repeating setup for all existing instances.

- After the installation report, choose full setup, edit one existing profile, or add/remove one.
- Profile picker shows each number, name, and color. Edit a name, an icon color, or both, with a focused review.
- Adding one asks for an unused profile number, name, and color. Unaffected launchers are left in place.
- `remove ID` retires only that launcher and preserves its sign-ins, chats, and data. `restore ID` restores the same identity; `list --removed` lists retained profiles.
- Removed numbers and existing data directories are reserved, preventing accidental account-state reuse. Removing the last profile also removes helper integration.
- Primary's normal deep-link handling is preserved when its managed launcher is removed; removed accounts are excluded from the connection chooser.
- macOS integration checks verify that targeted edits/removals preserve other launchers and profile data. Windows/Linux desktop integration remains experimental.

Credit to Edi Hasaj for the original independent-profile methodology.
