//go:build darwin && !purego

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework AVFoundation -framework Speech
#import <Foundation/Foundation.h>
#import <AVFoundation/AVFoundation.h>
#import <Speech/Speech.h>
#include <stdlib.h>

// Speaking instead of typing: the app hears it itself with Apple's speech
// recognition, in the language the person chose for the app (the system's
// Dictation would use the system's). On this Mac when it can (on-device);
// macOS asks for the microphone and speech recognition the first time.
// What is heard goes to Go (mycoHeard, in dictation_export_darwin.go) as
// the whole text so far; mycoHeardDone ends it. One listening at a time.

extern void mycoHeard(int sid, char *text, int final);
extern void mycoHeardDone(int sid, char *err);
extern void mycoHeardBreak(int sid);

static AVAudioEngine *mycoEngine;
static SFSpeechAudioBufferRecognitionRequest *mycoRequest;
static SFSpeechRecognitionTask *mycoTask;
static SFSpeechRecognizer *mycoRecognizer;
static int mycoSid;
// The person clicked to stop: the session's end is the listening's end.
static BOOL mycoStopping;
// Sessions in a row that heard nothing (silence): after a few, it stops.
static int mycoQuiet;
// Guards mycoRequest between the audio thread and the main queue.
static NSObject *mycoLock;

// Stops the microphone; the recognizer then gives its last words. (Main queue.)
static void mycoEndAudio(void) {
	if (mycoEngine) {
		[mycoEngine stop];
		[[mycoEngine inputNode] removeTapOnBus:0];
		[mycoEngine release];
		mycoEngine = nil;
	}
	@synchronized(mycoLock) {
		if (mycoRequest) [mycoRequest endAudio];
	}
}

// Lets go of the last listening's objects. (Main queue.)
static void mycoForget(void) {
	mycoEndAudio();
	if (mycoTask) { [mycoTask cancel]; [mycoTask release]; mycoTask = nil; }
	@synchronized(mycoLock) {
		if (mycoRequest) { [mycoRequest release]; mycoRequest = nil; }
	}
	if (mycoRecognizer) { [mycoRecognizer release]; mycoRecognizer = nil; }
}

// One recognition session on the running microphone. The recognizer ends a
// session when you pause (on this Mac it finalises the sentence), so while
// you have not stopped, the next one starts at once: a pause does not end
// the listening. (Main queue.)
static void mycoSession(int sid) {
	SFSpeechAudioBufferRecognitionRequest *req = [[SFSpeechAudioBufferRecognitionRequest alloc] init];
	req.shouldReportPartialResults = YES;
	if (@available(macOS 10.15, *)) {
		if (mycoRecognizer.supportsOnDeviceRecognition) req.requiresOnDeviceRecognition = YES;
	}
	@synchronized(mycoLock) {
		if (mycoRequest) { [mycoRequest endAudio]; [mycoRequest release]; }
		mycoRequest = req;
	}
	if (mycoTask) { [mycoTask release]; mycoTask = nil; }
	mycoTask = [[mycoRecognizer recognitionTaskWithRequest:req resultHandler:^(SFSpeechRecognitionResult *result, NSError *error) {
		BOOL final = result && [result isFinal];
		NSString *text = result ? [[result bestTranscription] formattedString] : nil;
		if ([text length]) mycoHeard(sid, (char *)[text UTF8String], final ? 1 : 0);
		if (!error && !final) return;
		BOOL any = [text length] > 0;
		NSString *msg = (error && !final) ? [[error localizedDescription] copy] : nil;
		dispatch_async(dispatch_get_main_queue(), ^{
			if (mycoSid != sid) { [msg release]; return; }
			// What it said in this session stays; the next goes after it.
			mycoHeardBreak(sid);
			if (mycoEngine && !mycoStopping) {
				mycoQuiet = any ? 0 : mycoQuiet + 1;
				if (mycoQuiet < 6) { mycoSession(sid); [msg release]; return; }
			}
			mycoHeardDone(sid, (msg && !mycoStopping && !any && mycoQuiet == 0) ? (char *)[msg UTF8String] : NULL);
			[msg release];
			mycoEndAudio();
		});
	}] retain];
}

static void mycoStart(int sid, NSString *loc) {
	if (!mycoLock) mycoLock = [[NSObject alloc] init];
	mycoForget();
	mycoStopping = NO;
	mycoQuiet = 0;
	mycoRecognizer = [[SFSpeechRecognizer alloc] initWithLocale:[NSLocale localeWithLocaleIdentifier:loc]];
	if (!mycoRecognizer || ![mycoRecognizer isAvailable]) { mycoHeardDone(sid, "unavailable"); return; }
	mycoEngine = [[AVAudioEngine alloc] init];
	AVAudioInputNode *input = [mycoEngine inputNode];
	AVAudioFormat *format = [input outputFormatForBus:0];
	[input installTapOnBus:0 bufferSize:1024 format:format block:^(AVAudioPCMBuffer *buffer, AVAudioTime *when) {
		@synchronized(mycoLock) {
			if (mycoRequest) [mycoRequest appendAudioPCMBuffer:buffer];
		}
	}];
	mycoSession(sid);
	[mycoEngine prepare];
	NSError *err = nil;
	if (![mycoEngine startAndReturnError:&err]) { mycoEndAudio(); mycoHeardDone(sid, "microphone"); return; }
}

static void mycoListen(int sid, const char *locale) {
	NSString *loc = [[NSString alloc] initWithUTF8String:locale];
	dispatch_async(dispatch_get_main_queue(), ^{
		mycoSid = sid;
		[SFSpeechRecognizer requestAuthorization:^(SFSpeechRecognizerAuthorizationStatus status) {
			if (status != SFSpeechRecognizerAuthorizationStatusAuthorized) { mycoHeardDone(sid, "speech-denied"); [loc release]; return; }
			[AVCaptureDevice requestAccessForMediaType:AVMediaTypeAudio completionHandler:^(BOOL granted) {
				if (!granted) { mycoHeardDone(sid, "mic-denied"); [loc release]; return; }
				dispatch_async(dispatch_get_main_queue(), ^{
					if (mycoSid == sid) mycoStart(sid, loc);
					else mycoHeardDone(sid, NULL);
					[loc release];
				});
			}];
		}];
	});
}

static void mycoStopListening(int sid) {
	dispatch_async(dispatch_get_main_queue(), ^{
		if (mycoSid != sid) return;
		mycoStopping = YES;
		mycoEndAudio();
	});
}
*/
import "C"

import (
	"sync"
	"unsafe"

	"github.com/chenhg5/myco/internal/console"
)

// A listening: where its text goes, and its sentences so far (the
// recognizer starts over after a pause).
type listening struct {
	h *console.Hearing
	u console.Utterances
}

var (
	hearingsMu sync.Mutex
	hearingsBy = map[int]*listening{}
	nextSid    int
)

// listen hears in lang (a BCP 47 tag such as zh-CN) and tells h what it
// heard; the function it returns stops the microphone (the last words
// still come).
var listen = func(lang string, h *console.Hearing) (func(), error) {
	hearingsMu.Lock()
	nextSid++
	sid := nextSid
	hearingsBy[sid] = &listening{h: h}
	hearingsMu.Unlock()
	cs := C.CString(lang)
	defer C.free(unsafe.Pointer(cs))
	C.mycoListen(C.int(sid), cs)
	return func() { C.mycoStopListening(C.int(sid)) }, nil
}

// dictate: on a Mac the app listens itself (listen).
var dictate func(lang string) error

// heard passes on what the recognizer has now, after what it said before
// a pause.
func heard(sid int, text string, final bool) {
	hearingsMu.Lock()
	l := hearingsBy[sid]
	var all string
	if l != nil {
		all = l.u.Add(text)
	}
	hearingsMu.Unlock()
	if l != nil {
		l.h.Heard(all, final)
	}
}

// heardBreak: a session of the recognizer ended (a pause); what it heard
// stays, and the next session's words go after it.
func heardBreak(sid int) {
	hearingsMu.Lock()
	defer hearingsMu.Unlock()
	if l := hearingsBy[sid]; l != nil {
		l.u.Commit()
	}
}

func heardDone(sid int) *console.Hearing {
	hearingsMu.Lock()
	defer hearingsMu.Unlock()
	l := hearingsBy[sid]
	delete(hearingsBy, sid)
	if l == nil {
		return nil
	}
	return l.h
}
