// Recognizes audio files with Apple's speech recognition, the way the myco
// App does (SFSpeechRecognizer), in one of several modes, and prints one
// JSON line per file: {"id", "mode", "text", "error"}.
//
//   recognize --locale zh-CN --mode hints-ondevice --hints hints.txt a.wav b.wav
//
// Modes: base-ondevice, hints-ondevice (contextualStrings), base-server,
// hints-server, lm-ondevice (macOS 14+: a custom language model built from
// the hints, SFCustomLanguageModelData). run.sh builds this with an
// Info.plist inside, so macOS can ask for the speech recognition permission.

import Foundation
import Speech

struct Line: Encodable { let id: String; let mode: String; let text: String; let error: String }

var locale = "zh-CN", mode = "base-ondevice", hintsPath = ""
var files: [String] = []
var args = CommandLine.arguments.dropFirst().makeIterator()
while let a = args.next() {
    switch a {
    case "--locale": locale = args.next() ?? locale
    case "--mode": mode = args.next() ?? mode
    case "--hints": hintsPath = args.next() ?? ""
    default: files.append(a)
    }
}
let hints = (try? String(contentsOfFile: hintsPath, encoding: .utf8))?
    .split(separator: "\n").map { $0.trimmingCharacters(in: .whitespaces) }.filter { !$0.isEmpty && !$0.hasPrefix("#") } ?? []

func emit(_ l: Line) {
    let d = try! JSONEncoder().encode(l)
    print(String(data: d, encoding: .utf8)!)
    fflush(stdout)
}

let sem = DispatchSemaphore(value: 0)
SFSpeechRecognizer.requestAuthorization { _ in sem.signal() }
sem.wait()
guard SFSpeechRecognizer.authorizationStatus() == .authorized else {
    FileHandle.standardError.write("speech recognition is not allowed: System Settings › Privacy & Security › Speech Recognition\n".data(using: .utf8)!)
    exit(3)
}
guard let rec = SFSpeechRecognizer(locale: Locale(identifier: locale)) else {
    FileHandle.standardError.write("no recognizer for \(locale)\n".data(using: .utf8)!)
    exit(3)
}

// A custom language model from the hints (macOS 14+), prepared once.
var lmConfig: Any? = nil
if mode == "lm-ondevice" {
    if #available(macOS 14, *) {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("myco-speech-eval-lm")
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let dataURL = dir.appendingPathComponent("lm.bin")
        let config = SFSpeechLanguageModel.Configuration(languageModel: dir.appendingPathComponent("lm"))
        let data = SFCustomLanguageModelData(locale: Locale(identifier: locale), identifier: "ai.myco.speech-eval", version: "1.0")
        for h in hints { data.insert(phraseCount: SFCustomLanguageModelData.PhraseCount(phrase: h, count: 20)) }
        let done = DispatchSemaphore(value: 0)
        var failure = ""
        Task {
            do {
                try await data.export(to: dataURL)
                try await SFSpeechLanguageModel.prepareCustomLanguageModel(for: dataURL, clientIdentifier: "ai.myco.speech-eval", configuration: config)
            } catch { failure = "\(error)" }
            done.signal()
        }
        done.wait()
        if !failure.isEmpty {
            for f in files { emit(Line(id: (f as NSString).lastPathComponent, mode: mode, text: "", error: "custom language model: " + failure)) }
            exit(0)
        }
        lmConfig = config
    } else {
        for f in files { emit(Line(id: (f as NSString).lastPathComponent, mode: mode, text: "", error: "needs macOS 14")) }
        exit(0)
    }
}

for f in files {
    let id = ((f as NSString).lastPathComponent as NSString).deletingPathExtension
    let req = SFSpeechURLRecognitionRequest(url: URL(fileURLWithPath: f))
    req.shouldReportPartialResults = false
    if #available(macOS 13, *) { req.addsPunctuation = true }
    let onDevice = mode.hasSuffix("ondevice")
    if onDevice && !rec.supportsOnDeviceRecognition {
        emit(Line(id: id, mode: mode, text: "", error: "no on-device recognition for \(locale)"))
        continue
    }
    req.requiresOnDeviceRecognition = onDevice
    if mode.hasPrefix("hints") { req.contextualStrings = hints }
    if #available(macOS 14, *), let c = lmConfig as? SFSpeechLanguageModel.Configuration {
        req.customizedLanguageModel = c
    }
    let done = DispatchSemaphore(value: 0)
    var text = "", err = ""
    let task = rec.recognitionTask(with: req) { result, error in
        if let r = result, r.isFinal { text = r.bestTranscription.formattedString; done.signal(); return }
        if let e = error { err = "\(e.localizedDescription)"; done.signal() }
    }
    if done.wait(timeout: .now() + 60) == .timedOut { task.cancel(); err = "timed out" }
    emit(Line(id: id, mode: mode, text: text, error: err))
}
