# Arcade Emulator

In the desktop App, the MAME mode detects an installed MAME or lets the user select its executable. The user opens a local ZIP whose filename matches the MAME short name (for example `snowbros.zip`). The app verifies the set against the installed MAME before opening a separate native game window. Put any required parent, BIOS and CHD files in the same ROM directory, following MAME's directory conventions. Close the game or use Stop in the app. MAME itself and arcade ROMs are obtained separately.

CHIP-8 mode runs a local 64 × 32 interpreter, with the original Orbit Courier demo. It supports modern or COSMAC VIP-style opcode behavior. Opening a ROM or changing compatibility starts a fresh machine. SUPER-CHIP and XO-CHIP are outside the CHIP-8 mode's scope.

Neither mode uploads ROM data or uses page records or agent actions. References: MAME's command-line documentation, Tobias V. I. Langhoff's CHIP-8 guide, and the Timendus test suite.

On Windows, leave iPhone arcade controller compatibility enabled when playing through the Remote Desktop arcade controller. It selects the Win32 keyboard provider for software-generated keys.
