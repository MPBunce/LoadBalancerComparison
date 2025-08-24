#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <signal.h>
#include <unistd.h>
#include <errno.h>
#include <sys/wait.h>

// Include our headers (we'll create these)
#include "config.h"
#include "server.h"
#include "load_balancer.h"
#include "health_checker.h"
#include "utils.h"

// Global variables for signal handling
static volatile int g_running = 1;
static server_t *g_server = NULL;
static health_checker_t *g_health_checker = NULL;

// Signal handler for graceful shutdown
void signal_handler(int sig) {
    switch (sig) {
        case SIGINT:
        case SIGTERM:
            printf("\n[INFO] Received signal %d, shutting down gracefully...\n", sig);
            g_running = 0;
            if (g_server) {
                g_server->running = 0;
            }
            break;
        case SIGHUP:
            printf("[INFO] Received SIGHUP, reloading configuration...\n");
            // TODO: Implement config reload
            break;
        default:
            break;
    }
}

// Setup signal handlers
int setup_signals() {
    struct sigaction sa;
    
    // Setup signal handler
    sa.sa_handler = signal_handler;
    sigemptyset(&sa.sa_mask);
    sa.sa_flags = 0;
    
    if (sigaction(SIGINT, &sa, NULL) == -1) {
        perror("sigaction SIGINT");
        return -1;
    }
    
    if (sigaction(SIGTERM, &sa, NULL) == -1) {
        perror("sigaction SIGTERM");
        return -1;
    }
    
    if (sigaction(SIGHUP, &sa, NULL) == -1) {
        perror("sigaction SIGHUP");
        return -1;
    }
    
    // Ignore SIGPIPE (common in network programming)
    signal(SIGPIPE, SIG_IGN);
    
    return 0;
}

// Print usage information
void print_usage(const char *program_name) {
    printf("Usage: %s [OPTIONS]\n", program_name);
    printf("\nOptions:\n");
    printf("  -c, --config FILE    Configuration file (default: config/loadbalancer.conf)\n");
    printf("  -p, --port PORT      Listen port (overrides config)\n");
    printf("  -d, --daemon         Run as daemon\n");
    printf("  -v, --verbose        Verbose logging\n");
    printf("  -h, --help           Show this help\n");
    printf("  --version            Show version information\n");
    printf("\nExamples:\n");
    printf("  %s                              # Use default config\n", program_name);
    printf("  %s -c /etc/lb.conf -p 8080     # Custom config and port\n", program_name);
    printf("  %s -d                          # Run as daemon\n", program_name);
}

// Print version information
void print_version() {
    printf("C Load Balancer v1.0.0\n");
    printf("Built on %s %s\n", __DATE__, __TIME__);
    printf("Features: epoll, health-checking, circuit-breaker, statistics\n");
}

// Daemonize the process
int daemonize() {
    pid_t pid = fork();
    
    if (pid < 0) {
        perror("fork failed");
        return -1;
    }
    
    // Parent process exits
    if (pid > 0) {
        exit(EXIT_SUCCESS);
    }
    
    // Child process continues
    if (setsid() < 0) {
        perror("setsid failed");
        return -1;
    }
    
    // Fork again to prevent acquiring controlling terminal
    pid = fork();
    if (pid < 0) {
        perror("second fork failed");
        return -1;
    }
    
    if (pid > 0) {
        exit(EXIT_SUCCESS);
    }
    
    // Change working directory
    if (chdir("/") < 0) {
        perror("chdir failed");
        return -1;
    }
    
    // Close standard file descriptors
    close(STDIN_FILENO);
    close(STDOUT_FILENO);
    close(STDERR_FILENO);
    
    return 0;
}

int main(int argc, char *argv[]) {
    // Default configuration
    const char *config_file = "config/loadbalancer.conf";
    int custom_port = -1;
    int daemon_mode = 0;
    int verbose = 0;
    
    // Parse command line arguments
    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "-c") == 0 || strcmp(argv[i], "--config") == 0) {
            if (++i < argc) {
                config_file = argv[i];
            } else {
                fprintf(stderr, "Error: -c requires a filename\n");
                return 1;
            }
        } else if (strcmp(argv[i], "-p") == 0 || strcmp(argv[i], "--port") == 0) {
            if (++i < argc) {
                custom_port = atoi(argv[i]);
                if (custom_port <= 0 || custom_port > 65535) {
                    fprintf(stderr, "Error: Invalid port number\n");
                    return 1;
                }
            } else {
                fprintf(stderr, "Error: -p requires a port number\n");
                return 1;
            }
        } else if (strcmp(argv[i], "-d") == 0 || strcmp(argv[i], "--daemon") == 0) {
            daemon_mode = 1;
        } else if (strcmp(argv[i], "-v") == 0 || strcmp(argv[i], "--verbose") == 0) {
            verbose = 1;
        } else if (strcmp(argv[i], "-h") == 0 || strcmp(argv[i], "--help") == 0) {
            print_usage(argv[0]);
            return 0;
        } else if (strcmp(argv[i], "--version") == 0) {
            print_version();
            return 0;
        } else {
            fprintf(stderr, "Error: Unknown option %s\n", argv[i]);
            print_usage(argv[0]);
            return 1;
        }
    }
    
    printf("🚀 Starting C Load Balancer...\n");
    
    // Setup signal handlers
    if (setup_signals() < 0) {
        fprintf(stderr, "Failed to setup signal handlers\n");
        return 1;
    }
    
    // Load configuration
    config_t config;
    if (config_load(&config, config_file) < 0) {
        fprintf(stderr, "Failed to load configuration from %s\n", config_file);
        return 1;
    }
    
    // Override port if specified
    if (custom_port > 0) {
        config.server_port = custom_port;
    }
    
    if (verbose) {
        config.log_level = LOG_DEBUG;
    }
    
    printf("📋 Configuration loaded:\n");
    printf("  - Listen port: %d\n", config.server_port);
    printf("  - Backend count: %d\n", config.backend_count);
    printf("  - Algorithm: %s\n", lb_algorithm_to_string(config.algorithm));
    printf("  - Max connections: %d\n", config.max_connections);
    
    // Daemonize if requested
    if (daemon_mode) {
        printf("🔄 Switching to daemon mode...\n");
        if (daemonize() < 0) {
            fprintf(stderr, "Failed to daemonize\n");
            return 1;
        }
        // After daemonization, stdout/stderr are closed
        // All logging should go through proper logging system
    }
    
    // Initialize load balancer
    loadbalancer_t lb;
    if (loadbalancer_init(&lb, &config) < 0) {
        fprintf(stderr, "Failed to initialize load balancer\n");
        return 1;
    }
    
    // Initialize server
    server_t server;
    g_server = &server;
    
    if (server_init(&server, &lb) < 0) {
        fprintf(stderr, "Failed to initialize server\n");
        loadbalancer_cleanup(&lb);
        return 1;
    }
    
    // Start health checker
    health_checker_t health_checker;
    g_health_checker = &health_checker;
    
    if (health_checker_init(&health_checker, &lb) < 0) {
        fprintf(stderr, "Failed to initialize health checker\n");
        server_cleanup(&server);
        loadbalancer_cleanup(&lb);
        return 1;
    }
    
    printf("✅ Load balancer started successfully!\n");
    printf("🌐 Listening on port %d\n", config.server_port);
    printf("📊 Stats available at http://localhost:%d/stats\n", config.server_port);
    printf("🏥 Health check at http://localhost:%d/health\n", config.server_port);
    
    // Main server loop
    int result = server_run(&server);
    
    printf("🛑 Server stopped, cleaning up...\n");
    
    // Cleanup in reverse order
    health_checker_cleanup(&health_checker);
    server_cleanup(&server);
    loadbalancer_cleanup(&lb);
    
    if (result < 0) {
        printf("❌ Server exited with error\n");
        return 1;
    }
    
    printf("✅ Clean shutdown complete\n");
    return 0;
}