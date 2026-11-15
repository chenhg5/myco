//go:build darwin && !purego

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework AVFoundation -framework Speech
#import <Foundation/Foundation.h>
#import <AVFoundation/AVFoundation.h>
#import <Speech/Speech.h>
#include <stdlib.h>

// A voice call's microphone and speaker. One audio engine does both, with
// the system's voice processing on (echo cancellation, noise suppression,
// automatic gain): what the assistant says is played through the same
// engine, so it is taken out of what the microphone hears, and the person
// can cut in while it talks. What is heard goes to Apple's speech
// recognition (on this Mac when it can), restarted after each pause for as
// long as the call is on. Text goes to Go (mycoVoiceHeard), the end of what
// was queued to play (mycoVoicePlayed), the end of the call (mycoVoiceEnded).

extern void mycoVoiceHeard(char *text, int final);
extern void mycoVoicePlayed(void);
extern void mycoVoiceEnded(char *err);

static AVAudioEngine *vEngine;
static AVAudioPlayerNode *vPlayer;
static AVAudioFormat *vPlayFormat;
static SFSpeechRecognizer *vRecognizer;
static SFSpeechAudioBufferRecognitionRequest *vRequest;
static SFSpeechRecognitionTask *vTask;
static NSObject *vLock;
static BOOL vOn;
// Buffers queued and not yet played, and which flush they belong to (a
// flush makes the older ones not count).
static int vPending;
static int vGeneration;

// One recognition session on the running microphone; the next starts when
// it ends (a pause), while the call is on. (Main queue.)
static void vSession(void) {
	if (!vOn) return;
	SFSpeechAudioBufferRecognitionRequest *req = [[SFSpeechAudioBufferRecognitionRequest alloc] init];
	req.shouldReportPartialResults = YES;
	if (@available(macOS 13, *)) req.addsPunctuation = YES;
	if (@available(macOS 10.15, *)) {
		if (vRecognizer.supportsOnDeviceRecognition) req.requiresOnDeviceRecognition = YES;
	}
	@synchronized(vLock) {
		if (vRequest) { [vRequest endAudio]; [vRequest release]; }
		vRequest = req;
	}
	if (vTask) { [vTask release]; vTask = nil; }
	vTask = [[vRecognizer recognitionTaskWithRequest:req resultHandler:^(SFSpeechRecognitionResult *result, NSError *error) {
		BOOL final = result && [result isFinal];
		NSString *text = result ? [[result bestTranscription] formattedString] : nil;
		if ([text length]) mycoVoiceHeard((char *)[text UTF8String], final ? 1 : 0);
		if (!error && !final) return;
		if (error) NSLog(@"myco voice: recognition ended: %@", error);
		dispatch_async(dispatch_get_main_queue(), ^{
			if (vOn && vTask && req == vRequest) vSession();
		});
	}] retain];
}

static void vTeardown(void) {
	vOn = NO;
	if (vEngine) {
		[vEngine stop];
		[[vEngine inputNode] removeTapOnBus:0];
	}
	@synchronized(vLock) {
		if (vRequest) { [vRequest endAudio]; [vRequest release]; vRequest = nil; }
	}
	if (vTask) { [vTask cancel]; [vTask release]; vTask = nil; }
	if (vPlayer) { [vPlayer stop]; [vPlayer release]; vPlayer = nil; }
	if (vPlayFormat) { [vPlayFormat release]; vPlayFormat = nil; }
	if (vEngine) { [vEngine release]; vEngine = nil; }
	if (vRecognizer) { [vRecognizer release]; vRecognizer = nil; }
	vPending = 0;
	vGeneration++;
}

static void vStartOnMain(NSString *loc) {
	if (!vLock) vLock = [[NSObject alloc] init];
	vTeardown();
	vRecognizer = [[SFSpeechRecognizer alloc] initWithLocale:[NSLocale localeWithLocaleIdentifier:loc]];
	if (!vRecognizer || ![vRecognizer isAvailable]) { mycoVoiceEnded("unavailable"); return; }
	vEngine = [[AVAudioEngine alloc] init];
	AVAudioInputNode *input = [vEngine inputNode];
	NSError *err = nil;
	// Echo cancellation and noise suppression: before the formats are read.
	if (@available(macOS 10.15, *)) {
		if (![input setVoiceProcessingEnabled:YES error:&err]) {
			NSLog(@"myco voice: voice processing unavailable: %@", err);
			err = nil;
		}
	}
	AVAudioFormat *inFormat = [input outputFormatForBus:0];
	NSLog(@"myco voice: microphone %@", inFormat);
	// With voice processing on, the microphone gives several channels (the
	// processed voice first); speech recognition hears nothing in that, so
	// it gets the first channel alone, as float mono.
	AVAudioFormat *mono = [[AVAudioFormat alloc] initWithCommonFormat:AVAudioPCMFormatFloat32 sampleRate:inFormat.sampleRate channels:1 interleaved:NO];
	[input installTapOnBus:0 bufferSize:1024 format:inFormat block:^(AVAudioPCMBuffer *buffer, AVAudioTime *when) {
		AVAudioPCMBuffer *out = nil;
		if (buffer.format.channelCount != 1 || buffer.format.commonFormat != AVAudioPCMFormatFloat32) {
			if (!buffer.floatChannelData) return;
			out = [[AVAudioPCMBuffer alloc] initWithPCMFormat:mono frameCapacity:buffer.frameLength];
			out.frameLength = buffer.frameLength;
			const float *src = buffer.floatChannelData[0];
			NSUInteger stride = buffer.format.isInterleaved ? buffer.format.channelCount : 1;
			float *dst = out.floatChannelData[0];
			for (AVAudioFrameCount i = 0; i < buffer.frameLength; i++) dst[i] = src[i * stride];
		}
		@synchronized(vLock) {
			if (vRequest) [vRequest appendAudioPCMBuffer:(out ? out : buffer)];
		}
		if (out) [out release];
	}];
	[mono release];
	// The assistant's voice: 24 kHz mono from Go, mixed into the output.
	vPlayer = [[AVAudioPlayerNode alloc] init];
	[vEngine attachNode:vPlayer];
	vPlayFormat = [[AVAudioFormat alloc] initWithCommonFormat:AVAudioPCMFormatFloat32 sampleRate:24000 channels:1 interleaved:NO];
	[vEngine connect:vPlayer to:[vEngine mainMixerNode] format:vPlayFormat];
	vOn = YES;
	[vEngine prepare];
	if (![vEngine startAndReturnError:&err]) { vTeardown(); mycoVoiceEnded("microphone"); return; }
	[vPlayer play];
	vSession();
}

static void mycoVoiceStart(const char *locale) {
	NSString *loc = [[NSString alloc] initWithUTF8String:locale];
	dispatch_async(dispatch_get_main_queue(), ^{
		[SFSpeechRecognizer requestAuthorization:^(SFSpeechRecognizerAuthorizationStatus status) {
			if (status != SFSpeechRecognizerAuthorizationStatusAuthorized) { mycoVoiceEnded("speech-denied"); [loc release]; return; }
			[AVCaptureDevice requestAccessForMediaType:AVMediaTypeAudio completionHandler:^(BOOL granted) {
				if (!granted) { mycoVoiceEnded("mic-denied"); [loc release]; return; }
				dispatch_async(dispatch_get_main_queue(), ^{
					vStartOnMain(loc);
					[loc release];
				});
			}];
		}];
	});
}

// Queues samples (16-bit little-endian mono at rate Hz) to be played.
static void mycoVoicePlay(const short *samples, int count, int rate) {
	if (count <= 0) return;
	NSData *data = [[NSData alloc] initWithBytes:samples length:(NSUInteger)count * 2];
	dispatch_async(dispatch_get_main_queue(), ^{
		if (!vOn || !vPlayer || rate != 24000) { [data release]; return; }
		AVAudioPCMBuffer *buf = [[AVAudioPCMBuffer alloc] initWithPCMFormat:vPlayFormat frameCapacity:(AVAudioFrameCount)count];
		buf.frameLength = (AVAudioFrameCount)count;
		const short *in = (const short *)[data bytes];
		float *out = buf.floatChannelData[0];
		for (int i = 0; i < count; i++) out[i] = (float)in[i] / 32768.0f;
		[data release];
		int gen = vGeneration;
		vPending++;
		[vPlayer scheduleBuffer:buf completionHandler:^{
			dispatch_async(dispatch_get_main_queue(), ^{
				if (gen != vGeneration) return;
				if (--vPending == 0) mycoVoicePlayed();
			});
		}];
		[buf release];
		if (![vPlayer isPlaying]) [vPlayer play];
	});
}

// Stops what is playing and drops what is queued (the person cut in).
static void mycoVoiceFlush(void) {
	dispatch_async(dispatch_get_main_queue(), ^{
		vGeneration++;
		vPending = 0;
		if (vPlayer) { [vPlayer stop]; [vPlayer play]; }
	});
}

static void mycoVoiceStop(void) {
	dispatch_async(dispatch_get_main_queue(), ^{ vTeardown(); });
}
*/
import "C"

import (
	"sync"
	"unsafe"

	"github.com/chenhg5/myco/internal/console"
)

// macVoice is the Mac's microphone and speaker for voice calls.
type macVoice struct {
	mu sync.Mutex
	ev console.VoiceEvents
}

var theVoice = &macVoice{}

func init() { voiceAudio = theVoice }

func (m *macVoice) Start(lang string, ev console.VoiceEvents) error {
	m.mu.Lock()
	m.ev = ev
	m.mu.Unlock()
	cs := C.CString(lang)
	defer C.free(unsafe.Pointer(cs))
	C.mycoVoiceStart(cs)
	return nil
}

func (m *macVoice) Play(pcm []byte, rate int) {
	n := len(pcm) / 2
	if n == 0 {
		return
	}
	C.mycoVoicePlay((*C.short)(unsafe.Pointer(&pcm[0])), C.int(n), C.int(rate))
}

func (m *macVoice) Flush() { C.mycoVoiceFlush() }

func (m *macVoice) Stop() {
	m.mu.Lock()
	m.ev = nil
	m.mu.Unlock()
	C.mycoVoiceStop()
}

func (m *macVoice) events() console.VoiceEvents {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ev
}
