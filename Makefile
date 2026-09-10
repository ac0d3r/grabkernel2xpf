EXTERNAL := external
BIN := $(EXTERNAL)/_bin
XPF := $(EXTERNAL)/XPF
GRAB := $(EXTERNAL)/libgrabkernel2
PARTIAL := $(EXTERNAL)/partial

XPF_LIB_SRC := $(wildcard $(XPF)/src/*.c)
XPF_CLI_SRC := $(wildcard $(XPF)/src/cli/*.c)
CHOMA_SRC := $(wildcard $(XPF)/external/ChOma/src/*.c)
IMG4_SRC := $(filter-out $(XPF)/external/img4lib/libvfs/vfs_lzvn.c, \
	$(wildcard $(XPF)/external/img4lib/lzss.c) \
	$(wildcard $(XPF)/external/img4lib/libvfs/*.c) \
	$(wildcard $(XPF)/external/img4lib/libDER/*.c))

XPF_CFLAGS := -O2 -framework Foundation -framework Security -lcompression \
	-I$(XPF)/external/ChOma/include \
	-I$(XPF)/external/img4lib \
	-DUSE_COMMONCRYPTO -DUSE_LIBCOMPRESSION -DiOS10 \
	-DDER_MULTIBYTE_TAGS=1 -D__unused="__attribute__((unused))" -DDER_TAG_SIZE=8 \
	-Wno-variadic-macros -Wno-multichar -Wno-four-char-constants -Wno-unused-parameter

DIST := grabkernel2xpf.tar

.PHONY: all deps build grabkernel xpf server pack clean

all: build

deps:
	git submodule update --init --recursive

xpf: deps
	mkdir -p $(BIN)
	rm -f $(BIN)/libxpf.dylib
	clang $(XPF_CFLAGS) -o $(BIN)/xpf_test $(XPF_LIB_SRC) $(XPF_CLI_SRC) $(CHOMA_SRC) $(IMG4_SRC)
	codesign -f -s - $(BIN)/xpf_test

grabkernel: deps
	$(MAKE) -C $(PARTIAL) TARGET=macos DISABLE_TESTS=1 output/macos/lib/libpartial.a
	mkdir -p $(GRAB)/_external/lib/macos
	cp -f $(PARTIAL)/output/macos/lib/libpartial.a $(GRAB)/_external/lib/macos/
	$(MAKE) -C $(GRAB) TARGET=macos DISABLE_TESTS=1 output/macos/lib/libgrabkernel2.a
	mkdir -p $(BIN)
	clang -fobjc-arc -O2 -mmacosx-version-min=11.0 \
		-I$(GRAB)/include -I$(GRAB)/_external/include \
		$(EXTERNAL)/grabkernel.m \
		$(GRAB)/output/macos/lib/libgrabkernel2.a \
		$(GRAB)/_external/lib/macos/libpartial.a \
		-framework Foundation -framework Security -lz \
		-o $(BIN)/grabkernel
	codesign -f -s - $(BIN)/grabkernel

server:
	go build -trimpath -ldflags="-s -w" -o server ./cmd/server

build: xpf grabkernel server

pack:
	mkdir -p data
	COPYFILE_DISABLE=1 tar --exclude='.*' -cf $(DIST) server web $(BIN) data install-launchd.sh

clean:
	rm -rf $(BIN) server $(DIST)
	-$(MAKE) -C $(XPF) clean
	-$(MAKE) -C $(GRAB) clean
	-$(MAKE) -C $(PARTIAL) clean
