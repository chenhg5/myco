# Arcade Emulator

This page runs a local CHIP-8 interpreter with the original Orbit Courier demonstration. The user can load a CHIP-8 program from their own device. The application has no page records or agent actions. ROM bytes are neither stored in the Hub nor sent to the agent.

Supported: standard 64 × 32 CHIP-8 with modern or COSMAC VIP-style opcode behavior. Opening a ROM or changing compatibility starts a fresh machine. SUPER-CHIP, XO-CHIP, MAME and FBNeo archives are outside this version's scope.

References: Tobias V. I. Langhoff’s CHIP-8 guide for instruction behavior, the Timendus test suite for interoperability tests, and EmulatorJS for the separate FBNeo/MAME architecture.
