//go:build darwin && !purego

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework AVFoundation -framework CoreAudio -framework AppKit
#import <Foundation/Foundation.h>
#import <AVFoundation/AVFoundation.h>
#import <CoreAudio/CoreAudio.h>
#import <CoreAudio/AudioHardwareTapping.h>
#import <CoreAudio/CATapDescription.h>
#import <AppKit/AppKit.h>
#include <stdlib.h>
#include <string.h>

// Recording for apps (myco.os audio.*): the microphone through an audio
// engine, and what the Mac plays, all of it or one app's, through a Core
// Audio process tap (macOS 14.2+): a tap on the processes, put in a private
// aggregate device whose input is read. Sound goes to Go as mono floats
// (mycoAudioSamples), by recording id.

extern void mycoAudioSamples(int id, float *data, int frames, double rate);

static NSMutableDictionary *maRecs; // id -> NSValue/AVAudioEngine
static NSObject *maLock;

static char *maErr(NSString *s) { return strdup([s UTF8String]); }

static void maInit(void) {
	static dispatch_once_t once;
	dispatch_once(&once, ^{
		maRecs = [[NSMutableDictionary alloc] init];
		maLock = [[NSObject alloc] init];
	});
}

// ---- the microphone ----

// Asks for the microphone if never asked; 1 when it may be used.
static int maMicAllowed(void) {
	AVAuthorizationStatus st = [AVCaptureDevice authorizationStatusForMediaType:AVMediaTypeAudio];
	if (st == AVAuthorizationStatusAuthorized) return 1;
	if (st != AVAuthorizationStatusNotDetermined) return 0;
	dispatch_semaphore_t sem = dispatch_semaphore_create(0);
	__block BOOL ok = NO;
	[AVCaptureDevice requestAccessForMediaType:AVMediaTypeAudio completionHandler:^(BOOL granted) {
		ok = granted;
		dispatch_semaphore_signal(sem);
	}];
	dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, 120 * NSEC_PER_SEC));
	dispatch_release(sem);
	return ok ? 1 : 0;
}

static char *maStartMic(int rid) {
	maInit();
	if (!maMicAllowed()) return maErr(@"permission_denied: myco 没有麦克风权限（系统设置 → 隐私与安全性 → 麦克风）");
	AVAudioEngine *e = [[AVAudioEngine alloc] init];
	AVAudioInputNode *in = [e inputNode];
	AVAudioFormat *f = [in outputFormatForBus:0];
	if ([f sampleRate] <= 0 || [f channelCount] == 0) {
		[e release];
		return maErr(@"not_found: 没有找到麦克风");
	}
	double rate = [f sampleRate];
	[in installTapOnBus:0 bufferSize:2048 format:f block:^(AVAudioPCMBuffer *buf, AVAudioTime *when) {
		int n = (int)[buf frameLength], ch = (int)[[buf format] channelCount];
		float * const *data = [buf floatChannelData];
		if (n <= 0 || !data) return;
		float *mono = malloc(sizeof(float) * n);
		if ([[buf format] isInterleaved]) {
			for (int i = 0; i < n; i++) {
				float s = 0;
				for (int c = 0; c < ch; c++) s += data[0][i * ch + c];
				mono[i] = s / ch;
			}
		} else {
			for (int i = 0; i < n; i++) {
				float s = 0;
				for (int c = 0; c < ch; c++) s += data[c][i];
				mono[i] = s / ch;
			}
		}
		mycoAudioSamples(rid, mono, n, rate);
		free(mono);
	}];
	[e prepare];
	NSError *err = nil;
	if (![e startAndReturnError:&err]) {
		[in removeTapOnBus:0];
		[e release];
		return maErr([NSString stringWithFormat:@"failed: 麦克风打不开：%@", [err localizedDescription]]);
	}
	@synchronized(maLock) { [maRecs setObject:e forKey:@(rid)]; }
	[e release];
	return NULL;
}

// ---- what the Mac plays: a process tap ----

@interface MATap : NSObject {
@public
	AudioObjectID tap, agg;
	AudioDeviceIOProcID proc;
}
@end
@implementation MATap
@end

static NSString *maDefaultOutputUID(void) {
	AudioObjectPropertyAddress a = { kAudioHardwarePropertyDefaultSystemOutputDevice, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain };
	AudioDeviceID dev = kAudioObjectUnknown;
	UInt32 size = sizeof(dev);
	if (AudioObjectGetPropertyData(kAudioObjectSystemObject, &a, 0, NULL, &size, &dev) != noErr || dev == kAudioObjectUnknown) return nil;
	a.mSelector = kAudioDevicePropertyDeviceUID;
	CFStringRef uid = NULL;
	size = sizeof(uid);
	if (AudioObjectGetPropertyData(dev, &a, 0, NULL, &size, &uid) != noErr || !uid) return nil;
	return [(NSString *)uid autorelease];
}

// The audio processes the HAL knows (macOS 14+): object id, pid, bundle id, playing.
typedef struct { AudioObjectID obj; pid_t pid; char bundle[256]; int playing; } MAProc;

static int maProcesses(MAProc **out) {
	*out = NULL;
	if (@available(macOS 14.0, *)) {
		AudioObjectPropertyAddress a = { kAudioHardwarePropertyProcessObjectList, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain };
		UInt32 size = 0;
		if (AudioObjectGetPropertyDataSize(kAudioObjectSystemObject, &a, 0, NULL, &size) != noErr || size == 0) return 0;
		int n = size / sizeof(AudioObjectID);
		AudioObjectID *ids = malloc(size);
		if (AudioObjectGetPropertyData(kAudioObjectSystemObject, &a, 0, NULL, &size, ids) != noErr) { free(ids); return 0; }
		n = size / sizeof(AudioObjectID);
		MAProc *ps = calloc(n, sizeof(MAProc));
		for (int i = 0; i < n; i++) {
			ps[i].obj = ids[i];
			AudioObjectPropertyAddress p = { kAudioProcessPropertyPID, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain };
			UInt32 sz = sizeof(pid_t);
			AudioObjectGetPropertyData(ids[i], &p, 0, NULL, &sz, &ps[i].pid);
			p.mSelector = kAudioProcessPropertyBundleID;
			CFStringRef b = NULL;
			sz = sizeof(b);
			if (AudioObjectGetPropertyData(ids[i], &p, 0, NULL, &sz, &b) == noErr && b) {
				CFStringGetCString(b, ps[i].bundle, sizeof(ps[i].bundle), kCFStringEncodingUTF8);
				CFRelease(b);
			}
			p.mSelector = kAudioProcessPropertyIsRunningOutput;
			UInt32 run = 0;
			sz = sizeof(run);
			AudioObjectGetPropertyData(ids[i], &p, 0, NULL, &sz, &run);
			ps[i].playing = run ? 1 : 0;
		}
		free(ids);
		*out = ps;
		return n;
	}
	return 0;
}

// The apps that have used sound (their helpers counted as theirs), as JSON:
// [{"bundle","name","playing"}].
static char *maApps(void) {
	@autoreleasepool {
		MAProc *ps = NULL;
		int n = maProcesses(&ps);
		NSMutableArray *list = [NSMutableArray array];
		NSMutableSet *seen = [NSMutableSet set];
		for (NSRunningApplication *app in [[NSWorkspace sharedWorkspace] runningApplications]) {
			if ([app activationPolicy] != NSApplicationActivationPolicyRegular) continue;
			NSString *bid = [app bundleIdentifier];
			if (![bid length] || [seen containsObject:bid] || [bid isEqualToString:[[NSBundle mainBundle] bundleIdentifier]]) continue;
			BOOL uses = NO, playing = NO;
			for (int i = 0; i < n; i++) {
				NSString *pb = [NSString stringWithUTF8String:ps[i].bundle];
				if (ps[i].pid == [app processIdentifier] || [pb isEqualToString:bid] || [pb hasPrefix:[bid stringByAppendingString:@"."]]) {
					uses = YES;
					if (ps[i].playing) playing = YES;
				}
			}
			if (!uses) continue;
			[seen addObject:bid];
			[list addObject:@{ @"bundle": bid, @"name": [app localizedName] ?: bid, @"playing": @(playing) }];
		}
		free(ps);
		NSData *d = [NSJSONSerialization dataWithJSONObject:list options:0 error:nil];
		NSString *s = [[[NSString alloc] initWithData:d encoding:NSUTF8StringEncoding] autorelease];
		return strdup([s UTF8String]);
	}
}

static char *maStartTap(int rid, const char *bundle) {
	maInit();
	if (@available(macOS 14.2, *)) {
		@autoreleasepool {
			NSString *outUID = maDefaultOutputUID();
			if (!outUID) return maErr(@"not_found: 没有找到声音输出设备");
			CATapDescription *d = nil;
			if (bundle && *bundle) {
				NSString *bid = [NSString stringWithUTF8String:bundle];
				NSMutableArray *objs = [NSMutableArray array];
				MAProc *ps = NULL;
				int n = maProcesses(&ps);
				for (int i = 0; i < n; i++) {
					NSString *pb = [NSString stringWithUTF8String:ps[i].bundle];
					if ([pb isEqualToString:bid] || [pb hasPrefix:[bid stringByAppendingString:@"."]]) [objs addObject:@(ps[i].obj)];
				}
				free(ps);
				if (![objs count]) return maErr([NSString stringWithFormat:@"not_found: %@ 还没有出过声音（开始会议后再录）", bid]);
				d = [[CATapDescription alloc] initStereoMixdownOfProcesses:objs];
			} else {
				d = [[CATapDescription alloc] initStereoGlobalTapButExcludeProcesses:@[]];
			}
			[d setUUID:[NSUUID UUID]];
			[d setPrivate:YES];
			[d setMuteBehavior:CATapUnmuted];
			[d setName:@"myco"];
			MATap *t = [[MATap alloc] init];
			OSStatus st = AudioHardwareCreateProcessTap(d, &t->tap);
			if (st != noErr) {
				[d release]; [t release];
				return maErr([NSString stringWithFormat:@"permission_denied: 录不了电脑的声音（%d）：在 系统设置 → 隐私与安全性 → 屏幕与系统录音 里允许 myco", (int)st]);
			}
			AudioObjectPropertyAddress fa = { kAudioTapPropertyFormat, kAudioObjectPropertyScopeGlobal, kAudioObjectPropertyElementMain };
			AudioStreamBasicDescription asbd;
			UInt32 sz = sizeof(asbd);
			memset(&asbd, 0, sizeof(asbd));
			AudioObjectGetPropertyData(t->tap, &fa, 0, NULL, &sz, &asbd);
			double rate = asbd.mSampleRate > 0 ? asbd.mSampleRate : 48000;
			NSDictionary *desc = @{
				@kAudioAggregateDeviceNameKey: @"myco recording",
				@kAudioAggregateDeviceUIDKey: [[NSUUID UUID] UUIDString],
				@kAudioAggregateDeviceMainSubDeviceKey: outUID,
				@kAudioAggregateDeviceIsPrivateKey: @YES,
				@kAudioAggregateDeviceIsStackedKey: @NO,
				@kAudioAggregateDeviceTapAutoStartKey: @YES,
				@kAudioAggregateDeviceSubDeviceListKey: @[ @{ @kAudioSubDeviceUIDKey: outUID } ],
				@kAudioAggregateDeviceTapListKey: @[ @{ @kAudioSubTapDriftCompensationKey: @YES, @kAudioSubTapUIDKey: [[d UUID] UUIDString] } ],
			};
			[d release];
			st = AudioHardwareCreateAggregateDevice((CFDictionaryRef)desc, &t->agg);
			if (st != noErr) {
				AudioHardwareDestroyProcessTap(t->tap);
				[t release];
				return maErr([NSString stringWithFormat:@"failed: 录音设备建不起来（%d）", (int)st]);
			}
			int tapCh = asbd.mChannelsPerFrame > 0 ? (int)asbd.mChannelsPerFrame : 2;
			BOOL interleaved = !(asbd.mFormatFlags & kAudioFormatFlagIsNonInterleaved);
			dispatch_queue_t q = dispatch_queue_create("myco.audio.tap", DISPATCH_QUEUE_SERIAL);
			st = AudioDeviceCreateIOProcIDWithBlock(&t->proc, t->agg, q, ^(const AudioTimeStamp *now, const AudioBufferList *inData, const AudioTimeStamp *inTime, AudioBufferList *outData, const AudioTimeStamp *outTime) {
				if (!inData || inData->mNumberBuffers == 0) return;
				// the tap's sound is the last stream (an output device with a
				// microphone, a headset, adds its own before it)
				if (interleaved) {
					const AudioBuffer *b = &inData->mBuffers[inData->mNumberBuffers - 1];
					for (int k = (int)inData->mNumberBuffers - 1; k >= 0; k--) {
						if ((int)inData->mBuffers[k].mNumberChannels == tapCh) { b = &inData->mBuffers[k]; break; }
					}
					int ch = b->mNumberChannels > 0 ? (int)b->mNumberChannels : 1;
					int n = b->mDataByteSize / (sizeof(float) * ch);
					if (n <= 0 || !b->mData) return;
					const float *src = b->mData;
					float *mono = malloc(sizeof(float) * n);
					for (int i = 0; i < n; i++) {
						float s = 0;
						for (int c = 0; c < ch; c++) s += src[i * ch + c];
						mono[i] = s / ch;
					}
					mycoAudioSamples(rid, mono, n, rate);
					free(mono);
				} else {
					int nb = (int)inData->mNumberBuffers, first = nb - tapCh;
					if (first < 0) first = 0;
					int n = inData->mBuffers[first].mDataByteSize / sizeof(float);
					if (n <= 0) return;
					float *mono = calloc(n, sizeof(float));
					int used = 0;
					for (int k = first; k < nb; k++) {
						const float *src = inData->mBuffers[k].mData;
						if (!src || (int)(inData->mBuffers[k].mDataByteSize / sizeof(float)) < n) continue;
						for (int i = 0; i < n; i++) mono[i] += src[i];
						used++;
					}
					if (used > 1) for (int i = 0; i < n; i++) mono[i] /= used;
					mycoAudioSamples(rid, mono, n, rate);
					free(mono);
				}
			});
			dispatch_release(q);
			if (st == noErr) st = AudioDeviceStart(t->agg, t->proc);
			if (st != noErr) {
				if (t->proc) AudioDeviceDestroyIOProcID(t->agg, t->proc);
				AudioHardwareDestroyAggregateDevice(t->agg);
				AudioHardwareDestroyProcessTap(t->tap);
				[t release];
				return maErr([NSString stringWithFormat:@"failed: 录音没能开始（%d）", (int)st]);
			}
			@synchronized(maLock) { [maRecs setObject:t forKey:@(rid)]; }
			[t release];
			return NULL;
		}
	}
	return maErr(@"unsupported: 录电脑的声音要 macOS 14.2 或更新");
}

static void maStop(int rid) {
	maInit();
	id r = nil;
	@synchronized(maLock) {
		r = [[maRecs objectForKey:@(rid)] retain];
		[maRecs removeObjectForKey:@(rid)];
	}
	if (!r) return;
	if ([r isKindOfClass:[AVAudioEngine class]]) {
		AVAudioEngine *e = r;
		[e stop];
		[[e inputNode] removeTapOnBus:0];
	} else if ([r isKindOfClass:[MATap class]]) {
		MATap *t = r;
		if (@available(macOS 14.2, *)) {
			AudioDeviceStop(t->agg, t->proc);
			AudioDeviceDestroyIOProcID(t->agg, t->proc);
			AudioHardwareDestroyAggregateDevice(t->agg);
			AudioHardwareDestroyProcessTap(t->tap);
		}
	}
	[r release];
}

static int maTapSupported(void) {
	if (@available(macOS 14.2, *)) return 1;
	return 0;
}
*/
import "C"

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"unsafe"

	"github.com/chenhg5/myco/internal/oscap"
)

// macAudio records for apps (oscap audio.*): the microphone, what the Mac
// plays ("system"), or one app's sound ("app:<bundle id>").
type macAudio struct{}

func init() { oscap.SetAudio(macAudio{}) }

var (
	audioMu   sync.Mutex
	audioNext int
	audioSink = map[int]func([]float32, int){}
)

func (macAudio) Sources(context.Context) ([]oscap.AudioSource, error) {
	out := []oscap.AudioSource{{ID: "mic", Kind: "mic", Name: "麦克风", Default: true}}
	if C.maTapSupported() == 0 {
		return out, nil
	}
	out = append(out, oscap.AudioSource{ID: "system", Kind: "system", Name: "电脑的声音"})
	cs := C.maApps()
	defer C.free(unsafe.Pointer(cs))
	var apps []struct {
		Bundle  string `json:"bundle"`
		Name    string `json:"name"`
		Playing bool   `json:"playing"`
	}
	_ = json.Unmarshal([]byte(C.GoString(cs)), &apps)
	for _, a := range apps {
		out = append(out, oscap.AudioSource{ID: "app:" + a.Bundle, Kind: "app", Name: a.Name, Bundle: a.Bundle, Playing: a.Playing})
	}
	return out, nil
}

func (macAudio) Capture(_ context.Context, id string, f func([]float32, int)) (func(), error) {
	audioMu.Lock()
	audioNext++
	rid := audioNext
	// (one at a time into f, as the interface says: the engine's and the
	// tap's callbacks each come from one thread, but a guard costs nothing)
	var one sync.Mutex
	audioSink[rid] = func(s []float32, rate int) { one.Lock(); f(s, rate); one.Unlock() }
	audioMu.Unlock()
	forget := func() { audioMu.Lock(); delete(audioSink, rid); audioMu.Unlock() }

	var cerr *C.char
	switch {
	case id == "mic":
		cerr = C.maStartMic(C.int(rid))
	case id == "system":
		cerr = C.maStartTap(C.int(rid), nil)
	case strings.HasPrefix(id, "app:") && len(id) > 4:
		cb := C.CString(strings.TrimPrefix(id, "app:"))
		cerr = C.maStartTap(C.int(rid), cb)
		C.free(unsafe.Pointer(cb))
	default:
		forget()
		return nil, &oscap.Error{Code: oscap.CodeInvalid, Message: "no such source: " + id + " (see audio.sources)"}
	}
	if cerr != nil {
		msg := C.GoString(cerr)
		C.free(unsafe.Pointer(cerr))
		forget()
		code, text, ok := strings.Cut(msg, ": ")
		if !ok {
			code, text = oscap.CodeFailed, msg
		}
		return nil, &oscap.Error{Code: code, Message: text}
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			C.maStop(C.int(rid))
			forget()
		})
	}, nil
}
