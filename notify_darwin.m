#import <AppKit/AppKit.h>
#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>

#include "_cgo_export.h"

@interface DBWNotificationDelegate : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation DBWNotificationDelegate

- (void)userNotificationCenter:(UNUserNotificationCenter *)center
       willPresentNotification:(UNNotification *)notification
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions))completionHandler {
    completionHandler(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionList | UNNotificationPresentationOptionSound);
}

- (void)userNotificationCenter:(UNUserNotificationCenter *)center
didReceiveNotificationResponse:(UNNotificationResponse *)response
         withCompletionHandler:(void (^)(void))completionHandler {
    NSString *query = response.notification.request.content.userInfo[@"query"];
    dispatch_async(dispatch_get_main_queue(), ^{
        [NSApp activateIgnoringOtherApps:YES];
    });
    dbwLog("notificaciones: clic recibido");
    if (query != nil) {
        dbwNotificationClicked((char *)[query UTF8String]);
    }
    completionHandler();
}

@end

static DBWNotificationDelegate *dbwDelegate;

bool dbwIsBundled(void) {
    return [[NSBundle mainBundle] bundleIdentifier] != nil;
}

void dbwSetupNotifications(void) {
    if (!dbwIsBundled()) {
        return;
    }
    dbwLog("notificaciones: configurando delegado");
    dispatch_async(dispatch_get_main_queue(), ^{
        dbwLog("notificaciones: pidiendo permiso");
        dbwDelegate = [DBWNotificationDelegate new];
        UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];
        center.delegate = dbwDelegate;
        [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
                              completionHandler:^(BOOL granted, NSError *_Nullable error) {
            if (error != nil) {
                dbwLog((char *)[[NSString stringWithFormat:@"notificaciones: error de permiso: %@", error] UTF8String]);
            }
            dbwAuthorizationResult(granted ? 1 : 0);
        }];
    });
}

void dbwNotify(const char *title, const char *body, const char *query) {
    UNMutableNotificationContent *content = [UNMutableNotificationContent new];
    content.title = [NSString stringWithUTF8String:title];
    content.body = [NSString stringWithUTF8String:body];
    content.sound = [UNNotificationSound defaultSound];
    content.userInfo = @{@"query" : [NSString stringWithUTF8String:query]};
    UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:[[NSUUID UUID] UUIDString]
                                                                          content:content
                                                                          trigger:nil];
    [[UNUserNotificationCenter currentNotificationCenter] addNotificationRequest:request
                                                           withCompletionHandler:^(NSError *_Nullable error) {
        if (error != nil) {
            dbwLog((char *)[[NSString stringWithFormat:@"notificaciones: no se pudo notificar: %@", error] UTF8String]);
        }
    }];
}
