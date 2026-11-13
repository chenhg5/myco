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

static AVAudioEngine *mycoEngine;
static SFSpeechAudioBufferRecognitionRequest *mycoRequest;
static SFSpeechRecognitionTask *mycoTask;
static SFSpeechRecognizer *mycoRecognizer;
static int mycoSid;

// Stops the microphone; the recognizer then gives its last words. (Main queue.)
static void mycoEndAudio(void) {
	if (mycoEngine) {
		[mycoEngine stop];
		[[mycoEngine inputNode] removeTapOnBus:0];
		[mycoEngine release];
		mycoEngine = nil;
	}
	if (mycoRequest) [mycoRequest endAudio];
}

// Lets go of the last listening's objects. (Main queue.)
static void mycoForget(void) {
	mycoEndAudio();
	if (mycoTask) { [mycoTask cancel]; [mycoTask release]; mycoTask = nil; }
	if (mycoRequest) { [mycoRequest release]; mycoRequest = nil; }
	if (mycoRecognizer) { [mycoRecognizer release]; mycoRecognizer = nil; }
}

static void mycoStart(int sid, NSString *loc) {
	mycoForget();
	mycoRecognizer = [[SFSpeechRecognizer alloc] initWithLocale:[NSLocale localeWithLocaleIdentifier:loc]];
	if (!mycoRecognizer || ![mycoRecognizer isAvailable]) { mycoHeardDone(sid, "unavailable"); return; }
	mycoRequest = [[SFSpeechAudioBufferRecognitionRequest alloc] init];
	mycoRequest.shouldReportPartialResults = YES;
	if (@available(macOS 10.15, *)) {
		if (mycoRecognizer.supportsOnDeviceRecognition) mycoRequest.requiresOnDeviceRecognition = YES;
	}
	mycoEngine = [[AVAudioEngine alloc] init];
	AVAudioInputNode *input = [mycoEngine inputNode];
	AVAudioFormat *format = [input outputFormatForBus:0];
	SFSpeechAudioBufferRecognitionRequest *request = mycoRequest;
	[input installTapOnBus:0 bufferSize:1024 format:format block:^(AVAudioPCMBuffer *buffer, AVAudioTime *when) {
		[request appendAudioPCMBuffer:buffer];
	}];
	[mycoEngine prepare];
	NSError *err = nil;
	if (![mycoEngine startAndReturnError:&err]) { mycoEndAudio(); mycoHeardDone(sid, "microphone"); return; }
	mycoTask = [[mycoRecognizer recognitionTaskWithRequest:mycoRequest resultHandler:^(SFSpeechRecognitionResult *result, NSError *error) {
		BOOL final = result && [result isFinal];
		if (result) mycoHeard(sid, (char *)[[[result bestTranscription] formattedString] UTF8String], final ? 1 : 0);
		if (error || final) {
			mycoHeardDone(sid, (error && !final) ? (char *)[[error localizedDescription] UTF8String] : NULL);
			dispatch_async(dispatch_get_main_queue(), ^{ if (mycoSid == sid) mycoEndAudio(); });
		}
	}] retain];
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
	dispatch_async(dispatch_get_main_queue(), ^{ if (mycoSid == sid) mycoEndAudio(); });
}
*/
import "C"

import (
	"sync"
	"unsafe"

	"github.com/chenhg5/myco/internal/console"
)

var (
	hearingsMu sync.Mutex
	hearingsBy = map[int]*console.Hearing{}
	nextSid    int
)

// listen hears in lang (a BCP 47 tag such as zh-CN) and tells h what it
// heard; the function it returns stops the microphone (the last words
// still come).
var listen = func(lang string, h *console.Hearing) (func(), error) {
	hearingsMu.Lock()
	nextSid++
	sid := nextSid
	hearingsBy[sid] = h
	hearingsMu.Unlock()
	cs := C.CString(lang)
	defer C.free(unsafe.Pointer(cs))
	C.mycoListen(C.int(sid), cs)
	return func() { C.mycoStopListening(C.int(sid)) }, nil
}

// dictate: on a Mac the app listens itself (listen).
var dictate func(lang string) error

func heard(sid int) *console.Hearing {
	hearingsMu.Lock()
	defer hearingsMu.Unlock()
	return hearingsBy[sid]
}

func heardDone(sid int) *console.Hearing {
	hearingsMu.Lock()
	defer hearingsMu.Unlock()
	h := hearingsBy[sid]
	delete(hearingsBy, sid)
	return h
}
