//
//  AppDelegate.m
//  midiconverser
//
//  Created by Rudolf Polzer on 10/18/24.
//

#import "AppDelegate.h"
#import "GameViewController.h"

@interface AppDelegate ()

@end

@implementation AppDelegate


- (BOOL)application:(UIApplication *)application didFinishLaunchingWithOptions:(NSDictionary *)launchOptions {
    return YES;
}


- (void)applicationWillResignActive:(UIApplication *)application {
    [[self ebitenViewContrller] suspendGame];
}


- (void)applicationDidEnterBackground:(UIApplication *)application {
}


- (void)applicationWillEnterForeground:(UIApplication *)application {
}


- (void)applicationDidBecomeActive:(UIApplication *)application {
    [[self ebitenViewContrller] resumeGame];
}


- (GameViewController *)ebitenViewContrller {
    return (GameViewController *)([[self window] rootViewController]);
}


@end
