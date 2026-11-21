//go:build darwin && !purego

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework AppKit -framework QuartzCore
#import <AppKit/AppKit.h>
#import <QuartzCore/QuartzCore.h>
#include <math.h>
extern void mycoScreenRegionSelected(int ident, double x, double y, double w, double h, double top, int cancelled);
extern void mycoScreenRegionStopped(int ident);

@class MycoRegionController;
@interface MycoRegionPanel : NSPanel
@end
@implementation MycoRegionPanel
- (BOOL)canBecomeKeyWindow { return YES; }
@end
@interface MycoRegionView : NSView
@property(nonatomic,assign) MycoRegionController *owner;
@property(nonatomic,assign) NSRect selection;
@property(nonatomic,assign) NSPoint anchor;
@property(nonatomic,retain) NSButton *recordButton;
@end
@interface MycoRegionController : NSObject
@property(nonatomic,assign) int ident;
@property(nonatomic,assign) BOOL zh;
@property(nonatomic,assign) BOOL recording;
@property(nonatomic,assign) BOOL finished;
@property(nonatomic,retain) NSMutableArray *panels;
@property(nonatomic,retain) NSArray *hidden;
@property(nonatomic,retain) NSStatusItem *status;
@property(nonatomic,assign) MycoRegionView *selected;
- (void)changed:(MycoRegionView*)v;
- (void)record:(id)sender;
- (void)cancel:(id)sender;
- (void)whole:(id)sender;
- (void)stop:(id)sender;
- (void)finish;
@end
@implementation MycoRegionView
- (BOOL)acceptsFirstResponder { return YES; }
- (void)resetCursorRects { [self addCursorRect:self.bounds cursor:[NSCursor crosshairCursor]]; }
- (void)drawRect:(NSRect)dirty {
 NSBezierPath *dim=[NSBezierPath bezierPathWithRect:self.bounds];
 if (!NSIsEmptyRect(self.selection)) [dim appendBezierPathWithRect:self.selection];
 [dim setWindingRule:NSEvenOddWindingRule];
 [[NSColor colorWithWhite:0 alpha:.48] setFill];[dim fill];
 if (!NSIsEmptyRect(self.selection)) {
  NSBezierPath *r=[NSBezierPath bezierPathWithRect:NSInsetRect(self.selection,.5,.5)];
  [[NSColor blackColor] setStroke];[r setLineWidth:3];[r stroke];
  [[NSColor whiteColor] setStroke];[r setLineWidth:1];[r stroke];
 }
}
- (void)mouseDown:(NSEvent*)e {
 self.anchor=[self convertPoint:e.locationInWindow fromView:nil];self.selection=NSZeroRect;
 [self.owner changed:self];
}
- (void)mouseDragged:(NSEvent*)e {
 NSPoint p=[self convertPoint:e.locationInWindow fromView:nil];
 self.selection=NSIntersectionRect(self.bounds,NSMakeRect(MIN(p.x,self.anchor.x),MIN(p.y,self.anchor.y),fabs(p.x-self.anchor.x),fabs(p.y-self.anchor.y)));
 [self.owner changed:self];
}
- (void)mouseUp:(NSEvent*)e { [self mouseDragged:e]; }
- (void)keyDown:(NSEvent*)e {if(e.keyCode==53)[self.owner cancel:nil];else if(e.keyCode==36)[self.owner record:nil];else [super keyDown:e];}
- (void)dealloc {self.recordButton=nil;[super dealloc];}
@end
@implementation MycoRegionController
- (void)changed:(MycoRegionView*)v {
 self.selected=v;
 for(NSWindow *p in self.panels){MycoRegionView *view=(MycoRegionView*)p.contentView;if(view!=v)view.selection=NSZeroRect;[view setNeedsDisplay:YES];view.recordButton.enabled=NSWidth(v.selection)>=16 && NSHeight(v.selection)>=16;}
}
- (void)whole:(id)sender {
 NSView *v=[sender superview];while(v && ![v isKindOfClass:[MycoRegionView class]])v=v.superview;
 if(v){((MycoRegionView*)v).selection=v.bounds;[self changed:(MycoRegionView*)v];}
}
- (void)record:(id)sender {
 if(self.finished || !self.selected || NSWidth(self.selected.selection)<16 || NSHeight(self.selected.selection)<16)return;
 self.finished=YES;
 NSRect r=[self.selected.window convertRectToScreen:self.selected.selection];
 CGFloat top=NSMaxY([NSScreen screens].firstObject.frame);
 for(NSWindow *p in self.panels)[p orderOut:nil];
 if(self.recording){
  self.status=[[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
  self.status.button.title=self.zh?@"■ 停止录制":@"■ Stop Recording";
  self.status.button.target=self;self.status.button.action=@selector(stop:);
  self.status.button.toolTip=self.zh?@"停止并返回剪辑":@"Stop and return to the editor";
 }
 mycoScreenRegionSelected(self.ident,r.origin.x,r.origin.y,r.size.width,r.size.height,top,0);
}
- (void)cancel:(id)sender {
 if(self.finished)return;self.finished=YES;
 for(NSWindow *p in self.panels)[p orderOut:nil];
 mycoScreenRegionSelected(self.ident,0,0,0,0,0,1);
}
- (void)stop:(id)sender {self.status.button.enabled=NO;mycoScreenRegionStopped(self.ident);}
- (void)finish {
 for(NSWindow *p in self.panels)[p close];
 [self.panels removeAllObjects];
 if(self.status){[[NSStatusBar systemStatusBar] removeStatusItem:self.status];self.status=nil;}
 if([NSApp isRunning]){for(NSWindow *w in self.hidden)[w orderFront:nil];[NSApp activateIgnoringOtherApps:YES];}
 self.hidden=nil;
}
- (void)dealloc {self.panels=nil;self.hidden=nil;self.status=nil;[super dealloc];}
@end
static NSMutableDictionary *mycoRegionSessions;
static NSButton *mycoRegionButton(NSString *title,NSRect frame,id target,SEL action){
 NSButton *b=[NSButton buttonWithTitle:title target:target action:action];b.frame=frame;b.bezelStyle=NSBezelStyleRounded;return b;
}
static void mycoBeginRegion(int ident,int zh,int recording){
 if(NSApp==nil){mycoScreenRegionSelected(ident,0,0,0,0,0,1);return;}
 dispatch_async(dispatch_get_main_queue(), ^{
  if(!mycoRegionSessions)mycoRegionSessions=[[NSMutableDictionary alloc]init];
  MycoRegionController *c=[[MycoRegionController alloc]init];c.ident=ident;c.zh=zh;c.recording=recording;c.panels=[NSMutableArray array];
  NSMutableArray *hidden=[NSMutableArray array];
  for(NSWindow *w in [NSApp windows])if(w.visible && ![w isKindOfClass:[MycoRegionPanel class]])[hidden addObject:w];
  c.hidden=hidden;for(NSWindow *w in hidden)[w orderOut:nil];
  mycoRegionSessions[@(ident)]=c;
  for(NSScreen *screen in [NSScreen screens]){
   MycoRegionPanel *p=[[MycoRegionPanel alloc]initWithContentRect:screen.frame styleMask:NSWindowStyleMaskBorderless backing:NSBackingStoreBuffered defer:NO];
   p.releasedWhenClosed=NO;p.opaque=NO;p.backgroundColor=[NSColor clearColor];p.level=NSScreenSaverWindowLevel;
   p.collectionBehavior=NSWindowCollectionBehaviorCanJoinAllSpaces|NSWindowCollectionBehaviorFullScreenAuxiliary;
   MycoRegionView *v=[[MycoRegionView alloc]initWithFrame:NSMakeRect(0,0,NSWidth(screen.frame),NSHeight(screen.frame))];v.owner=c;
   CGFloat width=MIN(640,NSWidth(v.bounds)-32);
   NSVisualEffectView *bar=[[NSVisualEffectView alloc]initWithFrame:NSMakeRect((NSWidth(v.bounds)-width)/2,40,width,86)];
   bar.material=NSVisualEffectMaterialHUDWindow;bar.state=NSVisualEffectStateActive;bar.wantsLayer=YES;bar.layer.cornerRadius=12;
   NSTextField *hint=[NSTextField labelWithString:zh?@"拖动框选区域，点击开始录制 · Esc 取消":@"Drag a region, then start recording · Esc to cancel"];
   hint.frame=NSMakeRect(16,50,width-32,22);hint.font=[NSFont systemFontOfSize:14 weight:NSFontWeightMedium];[bar addSubview:hint];
   NSButton *cancel=mycoRegionButton(zh?@"取消":@"Cancel",NSMakeRect(12,10,80,32),c,@selector(cancel:));cancel.keyEquivalent=@"\033";[bar addSubview:cancel];
   [bar addSubview:mycoRegionButton(zh?@"整个屏幕":@"Full Screen",NSMakeRect(100,10,116,32),c,@selector(whole:))];
   NSButton *record=mycoRegionButton(recording?(zh?@"开始录制":@"Start Recording"):(zh?@"选择此区域":@"Use Region"),NSMakeRect(width-160,10,148,32),c,@selector(record:));
   record.enabled=NO;record.keyEquivalent=@"\r";v.recordButton=record;[bar addSubview:record];[v addSubview:bar];[bar release];
   p.contentView=v;[v release];[p makeFirstResponder:v];[c.panels addObject:p];[p orderFrontRegardless];[p release];
  }
  [NSApp activateIgnoringOtherApps:YES];[[c.panels firstObject] makeKeyWindow];[c release];
 });
}
static void mycoEndRegion(int ident){dispatch_async(dispatch_get_main_queue(), ^{
 MycoRegionController *c=mycoRegionSessions[@(ident)];if(c){[c finish];[mycoRegionSessions removeObjectForKey:@(ident)];}
});}
*/
import "C"

import (
	"context"
	"sync"

	"github.com/chenhg5/myco/internal/oscap"
)

type regionAnswer struct {
	region    *oscap.ScreenRecordRegion
	cancelled bool
}
type nativeRegionSession struct {
	answer chan regionAnswer
	stop   func()
}

var nativeRegions = struct {
	sync.Mutex
	next     int
	sessions map[int]*nativeRegionSession
}{sessions: map[int]*nativeRegionSession{}}

func init() { screenPickerInstall = func() { oscap.SetScreenRegionPicker(pickNativeRegion) } }
func pickNativeRegion(ctx context.Context, stop func()) (*oscap.ScreenRecordRegion, func(), error) {
	nativeRegions.Lock()
	if len(nativeRegions.sessions) != 0 {
		nativeRegions.Unlock()
		return nil, nil, &oscap.Error{Code: oscap.CodeExists, Message: "a screen region selector is already open"}
	}
	nativeRegions.next++
	id := nativeRegions.next
	session := &nativeRegionSession{answer: make(chan regionAnswer, 1), stop: stop}
	nativeRegions.sessions[id] = session
	nativeRegions.Unlock()
	var once sync.Once
	cleanup := func() {
		once.Do(func() {
			C.mycoEndRegion(C.int(id))
			nativeRegions.Lock()
			delete(nativeRegions.sessions, id)
			nativeRegions.Unlock()
		})
	}
	zh := 0
	if screenPickerChinese.Load() {
		zh = 1
	}
	recording := 0
	if stop != nil {
		recording = 1
	}
	C.mycoBeginRegion(C.int(id), C.int(zh), C.int(recording))
	select {
	case <-ctx.Done():
		cleanup()
		return nil, nil, &oscap.Error{Code: oscap.CodeCancelled, Message: "region selection cancelled"}
	case r := <-session.answer:
		if r.cancelled || r.region == nil {
			cleanup()
			return nil, nil, &oscap.Error{Code: oscap.CodeCancelled, Message: "region selection cancelled"}
		}
		return r.region, cleanup, nil
	}
}
