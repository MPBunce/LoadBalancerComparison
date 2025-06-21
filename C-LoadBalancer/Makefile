# C Load Balancer Makefile
CC = clang
CFLAGS = -Wall -Wextra -Wstrict-prototypes -std=c99 -O2
DEBUG_FLAGS = -g -DDEBUG -fsanitize=address -fsanitize=undefined
LIBS = -lpthread

# Directories
SRCDIR = src
BINDIR = bin
OBJDIR = obj

# Source files
SOURCES = $(wildcard $(SRCDIR)/*.c)
OBJECTS = $(SOURCES:$(SRCDIR)/%.c=$(OBJDIR)/%.o)
TARGET = $(BINDIR)/C-LoadBalancer
DEBUG_TARGET = $(BINDIR)/C-LoadBalancer-debug

# Create directories if they don't exist
$(shell mkdir -p $(BINDIR) $(OBJDIR))

# Default target
all: $(TARGET)

# Production build
$(TARGET): $(OBJECTS)
	$(CC) $(CFLAGS) $(OBJECTS) -o $@ $(LIBS)

# Debug build
debug: CFLAGS += $(DEBUG_FLAGS)
debug: $(DEBUG_TARGET)

$(DEBUG_TARGET): $(OBJECTS)
	$(CC) $(CFLAGS) $(OBJECTS) -o $@ $(LIBS)

# Compile source files to object files
$(OBJDIR)/%.o: $(SRCDIR)/%.c
	$(CC) $(CFLAGS) -c $< -o $@

# Run targets
run: $(TARGET)
	@echo "Starting C Load Balancer..."
	./$(TARGET)

run-debug: $(DEBUG_TARGET)
	@echo "Starting C Load Balancer (Debug)..."
	./$(DEBUG_TARGET)

# Install target (copies to parent bin directory)
install: $(TARGET)
	cp $(TARGET) ../bin/C-LoadBalancer

# Clean build artifacts
clean:
	rm -rf $(OBJDIR) $(BINDIR)

# Show help
help:
	@echo "Available targets:"
	@echo "  all       - Build production version"
	@echo "  debug     - Build debug version with sanitizers"
	@echo "  run       - Build and run production version"
	@echo "  run-debug - Build and run debug version"
	@echo "  install   - Copy binary to parent bin directory"
	@echo "  clean     - Remove all build artifacts"
	@echo "  help      - Show this help message"

# Dependencies (basic dependency tracking)
$(OBJDIR)/main.o: $(SRCDIR)/main.c $(SRCDIR)/loadbalancer.h
$(OBJDIR)/server_pool.o: $(SRCDIR)/server_pool.c $(SRCDIR)/loadbalancer.h
$(OBJDIR)/http_parser.o: $(SRCDIR)/http_parser.c $(SRCDIR)/loadbalancer.h
$(OBJDIR)/proxy.o: $(SRCDIR)/proxy.c $(SRCDIR)/loadbalancer.h
$(OBJDIR)/utils.o: $(SRCDIR)/utils.c $(SRCDIR)/loadbalancer.h

.PHONY: all debug run run-debug install clean help