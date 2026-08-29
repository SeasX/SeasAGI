#import <Cocoa/Cocoa.h>
#import <dispatch/dispatch.h>
#include "_cgo_export.h"

@interface TrayDelegate : NSObject
@end

@implementation TrayDelegate
- (void)menuItemClicked:(id)sender {
    goTrayMenuClick((int)[(NSMenuItem *)sender tag]);
}
@end

static TrayDelegate *delegate = NULL;
static NSStatusItem *statusItem = NULL;

void createTray(void *iconData, int iconLen) {
	dispatch_async(dispatch_get_main_queue(), ^{
		static dispatch_once_t once;
		dispatch_once(&once, ^{
			delegate = [[TrayDelegate alloc] init];
		});

		statusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];

		if (iconData != NULL && iconLen > 0) {
			NSData *data = [NSData dataWithBytes:iconData length:(NSUInteger)iconLen];
			NSImage *img = [[NSImage alloc] initWithData:data];
			if (img) {
				[img setTemplate:YES];
				[statusItem.button setImage:img];
			}
		}
		if (statusItem.button.title.length == 0 && statusItem.button.image == nil) {
			[statusItem.button setTitle:@"S"];
		}
		[statusItem.button setToolTip:@"SeasAGI - 本地大模型通道切换客户端"];
		[statusItem setMenu:[[NSMenu alloc] init]];
	});
}

void addTrayItem(const char *title, int tag, int isSeparator, int isChecked) {
	if (statusItem == nil) return;
	dispatch_async(dispatch_get_main_queue(), ^{
		if (isSeparator) {
			[statusItem.menu addItem:[NSMenuItem separatorItem]];
		} else {
			NSString *label = [NSString stringWithUTF8String:title];
			NSMenuItem *mi = [[NSMenuItem alloc] initWithTitle:label action:@selector(menuItemClicked:) keyEquivalent:@""];
			[mi setTag:tag];
			[mi setTarget:delegate];
			if (isChecked) {
				[mi setState:NSControlStateValueOn];
			}
			[statusItem.menu addItem:mi];
		}
	});
}

void clearTrayMenu() {
	if (statusItem == nil) return;
	dispatch_async(dispatch_get_main_queue(), ^{
		[statusItem.menu removeAllItems];
	});
}

void destroyTray() {
	if (statusItem == nil) return;
	NSStatusItem *old = statusItem;
	statusItem = nil;
	dispatch_async(dispatch_get_main_queue(), ^{
		[[NSStatusBar systemStatusBar] removeStatusItem:old];
	});
}