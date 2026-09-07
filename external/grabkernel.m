#import <Foundation/Foundation.h>
#import "grabkernel.h"

#include <getopt.h>
#include <stdio.h>
#include <string.h>

static void usage(FILE *out) {
    fprintf(out,
            "usage: grabkernel --os OS --build BUILD --identifier ID --board BOARD --out DIR\n"
            "\n"
            "Download kernelcache (and SPTM/TXM when present) via libgrabkernel2.\n"
            "\n"
            "  --os           iOS or iPadOS\n"
            "  --build        build number, e.g. 21E219\n"
            "  --identifier   device identifier, e.g. iPhone15,2\n"
            "  --board        board config, e.g. d73ap\n"
            "  --out          output directory\n");
}

int main(int argc, char **argv) {
    @autoreleasepool {
        NSString *os = nil;
        NSString *build = nil;
        NSString *identifier = nil;
        NSString *board = nil;
        NSString *outDir = nil;

        static struct option opts[] = {
            {"os", required_argument, NULL, 'o'},
            {"build", required_argument, NULL, 'b'},
            {"identifier", required_argument, NULL, 'i'},
            {"board", required_argument, NULL, 'd'},
            {"out", required_argument, NULL, 'O'},
            {"help", no_argument, NULL, 'h'},
            {0, 0, 0, 0},
        };

        int c;
        while ((c = getopt_long(argc, argv, "o:b:i:d:O:h", opts, NULL)) != -1) {
            switch (c) {
                case 'o':
                    os = @(optarg);
                    break;
                case 'b':
                    build = @(optarg);
                    break;
                case 'i':
                    identifier = @(optarg);
                    break;
                case 'd':
                    board = @(optarg);
                    break;
                case 'O':
                    outDir = @(optarg);
                    break;
                case 'h':
                    usage(stdout);
                    return 0;
                default:
                    usage(stderr);
                    return 2;
            }
        }

        if (os.length == 0 || build.length == 0 || identifier.length == 0 || board.length == 0 ||
            outDir.length == 0) {
            usage(stderr);
            return 2;
        }

        BOOL ok = grab_images_for(os, build, identifier, board, outDir);
        if (!ok) {
            fprintf(stderr, "grabkernel: download failed\n");
            return 1;
        }
        printf("wrote images to %s\n", outDir.fileSystemRepresentation);
        return 0;
    }
}
