// Detect the locally installed MAME, verify the selected set, then follow the
// game process. Files are passed as command arguments on the desktop only.
import { useEffect, useRef, useState } from "react";
import { FolderOpen, Gamepad2, Play, Square } from "lucide-react";
import type { OSJob } from "../_lib/myco";
import { t as uiText } from "../_lib/i18n";
import { basename, dirname, has, os, osState, toOSError } from "../_lib/os";
import { mameArgs, mameVersion } from "./mame";

type Engine = { path: string; version: string };
type Phase = "setup" | "checking" | "running" | "stopping";

export function MamePanel() {
  const [ready, setReady] = useState<boolean>();
  const [engine, setEngine] = useState<Engine>();
  const [rom, setRom] = useState("");
  const [phase, setPhase] = useState<Phase>("setup");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [detail, setDetail] = useState("");
  const operation = useRef(false);
  const process = useRef<OSJob<{ code: number }>>();
  const alive = useRef(true);

  useEffect(() => {
    alive.current = true;
    if (osState === "ready") {
      void Promise.all(["fs.pick", "exec.run", "exec.spawn"].map(has))
        .then(caps => { if (alive.current) setReady(caps.every(Boolean)); });
    }
    return () => {
      alive.current = false;
      void process.current?.cancel().catch(() => undefined);
    };
  }, []);

  function cwd(path: string) {
    return /[/\\]/.test(path) ? dirname(path) : undefined;
  }

  async function findEngine(pick: boolean) {
    if (operation.current) return;
    operation.current = true;
    setBusy(true);
    setMessage(""); setDetail("");
    try {
      const candidates = pick
        ? (await os().fs.pick({ title: uiText("选择 MAME 程序") })).paths.slice(0, 1)
        : ["mame", "/opt/homebrew/bin/mame", "/usr/local/bin/mame", "/usr/games/mame"];
      for (const path of candidates) {
        if (!/^mame(?:\.exe)?$/i.test(basename(path))) {
          setMessage(uiText("请选择 mame 或 mame.exe 程序。"));
          return;
        }
        const result = await os().exec.run({ argv: [path, "-help"], cwd: cwd(path), timeout_ms: 5000 })
          .catch(() => undefined);
        const version = result?.code === 0 ? mameVersion(result.stdout) : undefined;
        if (version) { setEngine({ path, version }); return; }
      }
      setMessage(uiText("未找到可运行的 MAME。安装后可以手动选择程序。"));
    } catch (error) {
      if (toOSError(error).code !== "cancelled") setMessage(uiText("无法打开文件选择器。"));
    } finally {
      setBusy(false);
      operation.current = false;
    }
  }

  async function chooseROM() {
    if (operation.current) return;
    operation.current = true;
    setBusy(true);
    setMessage(""); setDetail("");
    try {
      const { paths } = await os().fs.pick({ title: uiText("选择 MAME ROM ZIP"), types: ["zip"] });
      if (!paths[0]) return;
      if (!mameArgs(paths[0])) { setMessage(uiText("请选择保留 MAME 原始短名的 ZIP 文件。")); return; }
      setRom(paths[0]);
    } catch (error) {
      if (toOSError(error).code !== "cancelled") setMessage(uiText("无法打开文件选择器。"));
    } finally {
      setBusy(false);
      operation.current = false;
    }
  }

  async function play() {
    const args = mameArgs(rom);
    if (operation.current || !engine || !args) return;
    operation.current = true;
    setPhase("checking");
    setMessage(""); setDetail("");
    try {
      const verified = await os().exec.run({ argv: [engine.path, ...args.verify], cwd: cwd(engine.path), timeout_ms: 30000 });
      if (!alive.current) return;
      if (verified.code !== 0 || verified.timed_out) {
        setMessage(uiText("ROM 校验未通过。请核对 MAME 版本，并把依赖的 BIOS 或父集放在同一文件夹里。"));
        setDetail((verified.stderr + "\n" + verified.stdout).trim().slice(-2800));
        return;
      }
      const job = os().exec.spawn({ argv: [engine.path, ...args.play], cwd: cwd(engine.path) });
      process.current = job;
      setPhase("running");
      let stderr = "", cancelled = false;
      job.on("stderr", text => { stderr = (stderr + String(text)).slice(-2800); });
      const result = await job.done.catch(error => {
        if (toOSError(error).code !== "cancelled") throw error;
        cancelled = true;
      });
      if (result && result.code !== 0) {
        setMessage(uiText("MAME 已退出，代码 {0}。", result.code));
        setDetail(stderr.trim());
      } else if (!cancelled) setMessage(uiText("游戏窗口已关闭。"));
    } catch (error) {
      setMessage(uiText("无法启动 MAME。"));
      setDetail(toOSError(error).message.slice(0, 2800));
    } finally {
      process.current = undefined;
      operation.current = false;
      setPhase("setup");
    }
  }

  async function stop() {
    if (!process.current) return;
    setPhase("stopping");
    try { await process.current.cancel(); }
    catch (error) {
      if (process.current) {
        setPhase("running");
        setMessage(uiText("无法停止 MAME，请在游戏窗口关闭它。"));
        setDetail(toOSError(error).message);
      }
    }
  }

  const locked = busy || phase !== "setup";
  const status = phase === "checking" ? uiText("正在校验 ROM…") : phase === "running" ? uiText("MAME 已启动。切换到游戏窗口开始玩。")
    : phase === "stopping" ? uiText("正在停止…") : message;
  return <main className="arc-mame">
    <section className="arc-mame-panel">
      <Gamepad2 size={28} aria-hidden="true"/>
      <h2>{uiText("MAME 街机")}</h2>
      <p>{uiText("用本机 MAME 打开街机 ROM ZIP，游戏会在独立窗口运行。")}</p>
      {(osState !== "ready" || ready === false) ? <p role="status">{uiText("MAME 模式需要支持本机程序的 myco 桌面 App。")}</p> :
        <div className="arc-mame-form">
          <div>
            <strong>{engine ? "MAME " + engine.version : uiText("连接 MAME")}</strong>
            {engine?.path && <p title={engine.path}>{engine.path}</p>}
            <div className="arc-tools">
              <button type="button" className="btn" disabled={!ready || locked} onClick={() => void findEngine(false)}>{uiText("查找 MAME")}</button>
              <button type="button" className="btn" disabled={!ready || locked} onClick={() => void findEngine(true)}>{uiText("选择 MAME 程序")}</button>
            </div>
          </div>
          <div>
            <strong>{rom ? basename(rom) : uiText("载入 ROM ZIP")}</strong>
            <div className="arc-tools">
              <button type="button" className="btn" disabled={!ready || locked} onClick={() => void chooseROM()}><FolderOpen size={15}/>{uiText("选择 MAME ROM ZIP")}</button>
              <button type="button" className="btn btn-primary" disabled={locked || !rom || !engine} onClick={() => void play()}><Play size={15}/>{uiText("启动游戏")}</button>
              {(phase === "running" || phase === "stopping") && <button type="button" className="btn" disabled={phase === "stopping"} onClick={() => void stop()}><Square size={14}/>{uiText("停止游戏")}</button>}
            </div>
          </div>
        </div>}
      {status && <p role="status">{status}</p>}
      {detail && <pre>{detail}</pre>}
      <p>{uiText("默认按 5 投币、1 开始、方向键移动，Ctrl 和 Alt 为动作键。")}</p>
      <div className="arc-mame-links"><a href="https://github.com/mamedev/mame/releases" target="_blank" rel="noreferrer">{uiText("下载官方 MAME")}</a>
        <a href="https://www.mamedev.org/roms/" target="_blank" rel="noreferrer">{uiText("MAME 官方试玩 ROM")}</a></div>
    </section>
  </main>;
}
