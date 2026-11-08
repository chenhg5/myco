"""Synthesize the promo's music and sound effects (numpy only, no samples).

    python3 music.py <out dir>   -> bgm.wav (68 s, 120 BPM) and sfx/<name>.wav

The track follows script.json's sections: a sparse intro (0-8 s), a build with
a riser and snare roll (8-14 s), a drop at 14 s, a full groove, a lift into the
montage at 50 s, and a final hit at 58 s. A minor: Am - F - C - G, one chord a bar.
"""
import os, sys, wave
import numpy as np

SR = 48000
BPM = 120
BEAT = 60 / BPM
BAR = 4 * BEAT
TOTAL = 68.0
rng = np.random.default_rng(7)


def write(path, x):
    x = np.clip(x, -1, 1)
    st = np.stack([x, x], 1) if x.ndim == 1 else x
    with wave.open(path, "wb") as w:
        w.setnchannels(2); w.setsampwidth(2); w.setframerate(SR)
        w.writeframes((st * 32767).astype("<i2").tobytes())


def t(n):
    return np.arange(n) / SR


def env(n, a=0.002, d=0.2, curve=4.0):
    e = np.ones(n)
    na = max(1, int(a * SR)); e[:na] = np.linspace(0, 1, na)
    tail = np.arange(n - na) / SR
    e[na:] = np.exp(-curve * tail / max(d, 1e-3))
    return e


def lowpass(x, cutoff):
    # one-pole low-pass, cutoff may be an array (sweeps)
    c = np.broadcast_to(np.asarray(cutoff, float), x.shape)
    a = 1 - np.exp(-2 * np.pi * c / SR)
    y = np.empty_like(x); s = 0.0
    for i in range(len(x)):
        s += a[i] * (x[i] - s); y[i] = s
    return y


def highpass(x, cutoff):
    return x - lowpass(x, cutoff)


def note(midi):
    return 440.0 * 2 ** ((midi - 69) / 12)


# ---------- instruments ----------
def kick(n=int(0.45 * SR), punch=1.0):
    tt = t(n)
    f = 45 + 110 * np.exp(-tt * 28)
    ph = 2 * np.pi * np.cumsum(f) / SR
    body = np.sin(ph) * env(n, 0.001, 0.32, 5)
    click = rng.standard_normal(n) * env(n, 0.0005, 0.01, 6) * 0.3
    return (body + click) * punch


def clap(n=int(0.3 * SR)):
    nz = highpass(rng.standard_normal(n), 900)
    e = np.zeros(n)
    for off in (0, 0.011, 0.022):
        i = int(off * SR); e[i:] += env(n - i, 0.0005, 0.03 if off < 0.02 else 0.18, 4)
    return nz * e * 0.45


def snare(n=int(0.22 * SR)):
    nz = highpass(rng.standard_normal(n), 1500) * env(n, 0.0005, 0.12, 5)
    tone = np.sin(2 * np.pi * 190 * t(n)) * env(n, 0.0005, 0.06, 5)
    return (nz * 0.55 + tone * 0.35)


def hat(open_=False):
    n = int((0.22 if open_ else 0.06) * SR)
    nz = highpass(rng.standard_normal(n), 7000)
    return nz * env(n, 0.0005, 0.18 if open_ else 0.035, 4) * (0.22 if open_ else 0.16)


def saw(freq, n, detune=0.0):
    tt = t(n); x = 0
    for d in (-detune, 0, detune):
        ph = (tt * freq * (1 + d)) % 1.0
        x = x + (2 * ph - 1)
    return x / 3


def pluck(freq, n=int(0.32 * SR), bright=3500):
    x = saw(freq, n, 0.004) * env(n, 0.002, 0.22, 5)
    return lowpass(x, bright * env(n, 0.001, 0.15, 3) + 400) * 0.5


def bass(freq, n):
    x = saw(freq, n, 0.002) + 0.6 * np.sin(2 * np.pi * freq / 2 * t(n))
    return lowpass(x, 700) * env(n, 0.004, 0.25, 2.5) * 0.55


def pad(freqs, n, cutoff=1800):
    x = sum(saw(f, n, 0.006) for f in freqs) / len(freqs)
    e = np.minimum(1, t(n) / 0.25) * np.minimum(1, (n / SR - t(n)) / 0.3)
    return lowpass(x, cutoff) * e * 0.3


CHORDS = [  # Am F C G (root midi, triad)
    (45, [57, 60, 64]), (41, [57, 60, 65]), (48, [55, 60, 64]), (43, [55, 59, 62]),
]
ARP = [0, 2, 1, 2, 0, 2, 1, 2, 0, 2, 1, 2, 0, 1, 2, 1]


def build():
    n = int(TOTAL * SR)
    drums, bassl, music, fx = (np.zeros(n) for _ in range(4))

    def put(buf, x, at, gain=1.0):
        i = int(at * SR)
        if i >= n: return
        j = min(n, i + len(x)); buf[i:j] += x[: j - i] * gain

    bars = int(TOTAL / BAR)
    for b in range(bars):
        t0 = b * BAR; sec = t0
        root, triad = CHORDS[b % 4]
        full = 14 <= sec < 58
        intro = sec < 8
        build_ = 8 <= sec < 14
        ending = sec >= 58
        # pad, opening up after the drop
        cut = 900 if intro else 1300 if build_ else 2600 if full else 1600
        if sec < 64:
            put(music, pad([note(m) for m in triad], int(BAR * SR), cut), t0, 0.9 if full else 1.6 if intro else 0.8)
        for k in range(4):  # beats
            tb = t0 + k * BEAT
            if full or (build_ and k % 2 == 0) or (ending and tb < 64 and k == 0):
                put(drums, kick(punch=1.0 if full else 0.7), tb)
            if full and k in (1, 3):
                put(drums, clap(), tb, 0.9)
            for h in range(2):  # 8th hats
                th = tb + h * BEAT / 2
                if intro and h == 0: continue
                if sec < 64:
                    put(drums, hat(open_=(full and h == 1)), th, 1.5 if intro else 1.0)
            if 50 <= sec < 58:  # montage: extra 16th hats
                for q in (0.25, 0.75):
                    put(drums, hat(), tb + q * BEAT, 0.6)
            if full and not (46 <= sec < 50):  # bass, 8ths, ducked by the kick
                for h in range(2):
                    put(bassl, bass(note(root), int(BEAT / 2 * SR)), tb + h * BEAT / 2, 0.9 if h else 0.55)
        # 16th pluck arpeggio from the build on
        if sec >= 8 and sec < 64:
            for s in range(16):
                ts = t0 + s * BEAT / 4
                if build_ and s % 2: continue
                m = triad[ARP[s]] + (12 if (s // 4) % 2 and full else 0)
                put(music, pluck(note(m), bright=2200 if build_ else 4200), ts, 0.33 if full else 0.24)

    # build 8-14: snare roll accelerating into the drop, noise riser
    tb = 12.0
    for i in range(16):
        put(drums, snare(), 12.0 + i * BEAT / 4, 0.25 + 0.04 * i)
    rn = int(6 * SR); tt = t(rn)
    riser = highpass(rng.standard_normal(rn), 400)
    riser = lowpass(riser, 600 + 9000 * (tt / 6) ** 2) * (tt / 6) ** 2 * 0.45
    put(fx, riser, 8.0)
    # lift into the montage (48-50)
    rn2 = int(2 * SR); tt2 = t(rn2)
    put(fx, lowpass(highpass(rng.standard_normal(rn2), 500), 800 + 8000 * (tt2 / 2) ** 2) * (tt2 / 2) ** 2 * 0.35, 48.0)
    for i in range(8):
        put(drums, snare(), 49.0 + i * BEAT / 4, 0.25 + 0.06 * i)
    # impacts: drop, montage, end
    for at, g in ((14.0, 1.0), (50.0, 0.7), (58.0, 1.0)):
        put(fx, impact(), at, g)

    # sidechain: duck pads/bass/plucks on each kick after the drop
    duck = np.ones(n)
    for b in range(int(TOTAL / BEAT)):
        tb = b * BEAT
        if 14 <= tb < 58:
            i = int(tb * SR); m = int(0.22 * SR)
            duck[i:i + m] = np.minimum(duck[i:i + m], 0.35 + 0.65 * (np.arange(m) / m) ** 0.6)
    mix = drums * 0.9 + (bassl + music) * duck + fx
    # fade the last bars
    fade = np.ones(n); fs = int(62 * SR); fade[fs:] = np.linspace(1, 0, n - fs) ** 1.5
    mix *= fade
    return mix / (np.max(np.abs(mix)) + 1e-9) * 0.89


def impact(n=int(2.6 * SR)):
    tt = t(n)
    boom = np.sin(2 * np.pi * np.cumsum(35 + 60 * np.exp(-tt * 9)) / SR) * env(n, 0.001, 1.2, 3)
    crash = highpass(rng.standard_normal(n), 3000) * env(n, 0.001, 1.6, 3) * 0.35
    return (boom * 0.9 + crash) * 0.8


# ---------- sound effects ----------
def sfx():
    out = {}
    n = int(0.45 * SR); tt = t(n)
    out["whoosh"] = lowpass(rng.standard_normal(n), 300 + 5000 * np.sin(np.pi * tt / tt[-1]) ** 2) * np.sin(np.pi * tt / tt[-1]) ** 2 * 0.6
    n = int(0.08 * SR)
    out["click"] = (highpass(rng.standard_normal(n), 2500) * env(n, 0.0003, 0.012, 5) * 0.6
                    + np.sin(2 * np.pi * 1800 * t(n)) * env(n, 0.0003, 0.01, 5) * 0.3)
    n = int(0.18 * SR); tt = t(n)
    out["pop"] = np.sin(2 * np.pi * np.cumsum(500 + 900 * np.exp(-tt * 30)) / SR) * env(n, 0.001, 0.08, 4) * 0.55
    n = int(0.05 * SR)
    out["tick"] = highpass(rng.standard_normal(n), 3500) * env(n, 0.0003, 0.008, 5) * 0.35
    n = int(0.9 * SR); tt = t(n)
    out["ding"] = (np.sin(2 * np.pi * 1318.5 * tt) * 0.5 + np.sin(2 * np.pi * 1975.5 * tt) * 0.25) * env(n, 0.002, 0.6, 4) * 0.45
    n = int(0.35 * SR); tt = t(n)
    out["send"] = lowpass(rng.standard_normal(n), 800 + 6000 * (tt / tt[-1])) * np.sin(np.pi * tt / tt[-1]) * 0.45 \
        + np.sin(2 * np.pi * np.cumsum(600 + 1400 * tt / tt[-1]) / SR) * env(n, 0.01, 0.25, 3) * 0.15
    n = int(0.5 * SR); tt = t(n)
    out["zip"] = np.sin(2 * np.pi * np.cumsum(400 + 2400 * (tt / tt[-1]) ** 1.5) / SR) * np.sin(np.pi * tt / tt[-1]) * 0.25
    n = int(1.6 * SR); tt = t(n)
    out["shimmer"] = sum(np.sin(2 * np.pi * f * tt) for f in (1760, 2217, 2637, 3520)) / 4 * env(n, 0.05, 1.0, 3) * 0.35
    return out


if __name__ == "__main__":
    out = sys.argv[1]
    os.makedirs(f"{out}/sfx", exist_ok=True)
    write(f"{out}/bgm.wav", build())
    for k, v in sfx().items():
        write(f"{out}/sfx/{k}.wav", v)
    print("ok", out)
