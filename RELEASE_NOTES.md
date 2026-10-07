Setup wizard, named launchers, and five profile icon colors.

- `wizard` (also the default command) guides you through profile count, names, icon colors, Dock pins, and a final review. Guided `add` customizes only new profiles; commands with `--count` remain unattended.
- `rename ID NAME` updates Spotlight/Raycast, Dock launcher, helper menu, and chooser names while preserving profile IDs and sign-ins.
- macOS launchers now live directly in `~/Applications`, making additional profiles discoverable. Existing Dock pin positions are migrated; Edi launcher originals are kept as recoverable backups.
- `icons` lists white, yellow, blue, purple, and teal. `icon ID COLOR` changes one profile and persists through updates. Dark backgrounds use a white OpenAI mark; white/yellow use dark lines.
- Colored icons appear in the existing macOS helper menu and chooser, plus OS launcher icons. Windows/Linux icon adapters remain experimental.
- Conflicting or filesystem-unsafe launcher names are rejected without replacing unrelated apps.

Credit to Edi Hasaj for the original independent-profile methodology. OpenAI owns the Blossom mark. Read README.md for platform limitations and authentication verification status.
