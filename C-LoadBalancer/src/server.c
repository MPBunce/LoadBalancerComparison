#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <errno.h>
#include <fcntl.h>
#include <sys/socket.h>
#include <sys/epoll.h>
#include <netinet/in.h>
#include <arpa/inet.h>
#include <time.h>

#include "server.h"
#include "load_balancer.h"
#include "http_parser.h"
#include "utils.h"
#include "stats.h"

#define MAX_EVENTS 1024
#define BUFFER_SIZE 8192

// Set socket to non-blocking mode
static int set_nonblocking(int fd) {
    int flags = fcntl(fd, F_GETFL, 0);
    if (flags == -1) {
        perror("fcntl F_GETFL");
        return -1;
    }
    
    if (fcntl(fd, F_SETFL, flags | O_NONBLOCK) == -1) {
        perror("fcntl F_SETFL");
        return -1;
    }
    
    return 0;
}

// Create and bind listening socket
static int create_listen_socket(int port) {
    int listen_fd = socket(AF_INET, SOCK_STREAM, 0);
    if (listen_fd == -1) {
        perror("socket");
        return -1;
    }
    
    // Enable address reuse
    int reuse = 1;
    if (setsockopt(listen_fd, SOL_SOCKET, SO_REUSEADDR, &reuse, sizeof(reuse)) == -1) {
        perror("setsockopt SO_REUSEADDR");
        close(listen_fd);
        return -1;
    }
    
    // Bind to address
    struct sockaddr_in addr;
    memset(&addr, 0, sizeof(addr));
    addr.sin_family = AF_INET;
    addr.sin_addr.s_addr = INADDR_ANY;
    addr.sin_port = htons(port);
    
    if (bind(listen_fd, (struct sockaddr*)&addr, sizeof(addr)) == -1) {
        perror("bind");
        close(listen_fd);
        return -1;
    }
    
    // Start listening
    if (listen(listen_fd, SOMAXCONN) == -1) {
        perror("listen");
        close(listen_fd);
        return -1;
    }
    
    // Set non-blocking
    if (set_nonblocking(listen_fd) == -1) {
        close(listen_fd);
        return -1;
    }
    
    return listen_fd;
}

// Accept new client connection
static void handle_accept(server_t *server) {
    struct sockaddr_in client_addr;
    socklen_t client_len = sizeof(client_addr);
    
    while (1) {
        int client_fd = accept(server->listen_fd, (struct sockaddr*)&client_addr, &client_len);
        if (client_fd == -1) {
            if (errno == EAGAIN || errno == EWOULDBLOCK) {
                break; // No more connections to accept
            }
            perror("accept");
            break;
        }
        
        // Check connection limit
        if (server->active_connections >= server->max_connections) {
            printf("Connection limit reached, rejecting client\n");
            close(client_fd);
            continue;
        }
        
        // Set client socket non-blocking
        if (set_nonblocking(client_fd) == -1) {
            close(client_fd);
            continue;
        }
        
        // Find available connection slot
        connection_t *conn = NULL;
        for (int i = 0; i < server->max_connections; i++) {
            if (server->connections[i].client_fd == -1) {
                conn = &server->connections[i];
                break;
            }
        }
        
        if (!conn) {
            printf("No connection slots available\n");
            close(client_fd);
            continue;
        }
        
        // Initialize connection
        memset(conn, 0, sizeof(connection_t));
        conn->client_fd = client_fd;
        conn->backend_fd = -1;
        conn->state = CONN_READING_REQUEST;
        conn->client_addr = client_addr;
        conn->start_time = time(NULL);
        conn->request_buffer = malloc(BUFFER_SIZE);
        conn->response_buffer = malloc(BUFFER_SIZE);
        
        if (!conn->request_buffer || !conn->response_buffer) {
            printf("Memory allocation failed for connection buffers\n");
            free(conn->request_buffer);
            free(conn->response_buffer);
            close(client_fd);
            continue;
        }
        
        // Add to epoll
        struct epoll_event ev;
        ev.events = EPOLLIN | EPOLLET; // Edge-triggered
        ev.data.ptr = conn;
        
        if (epoll_ctl(server->epoll_fd, EPOLL_CTL_ADD, client_fd, &ev) == -1) {
            perror("epoll_ctl ADD client");
            free(conn->request_buffer);
            free(conn->response_buffer);
            close(client_fd);
            conn->client_fd = -1;
            continue;
        }
        
        server->active_connections++;
        
        char client_ip[INET_ADDRSTRLEN];
        inet_ntop(AF_INET, &client_addr.sin_addr, client_ip, INET_ADDRSTRLEN);
        printf("New client connection from %s:%d (fd=%d)\n", 
               client_ip, ntohs(client_addr.sin_port), client_fd);
    }
}

// Read HTTP request from client
static int handle_client_read(server_t *server, connection_t *conn) {
    char buffer[BUFFER_SIZE];
    ssize_t bytes_read;
    
    while ((bytes_read = read(conn->client_fd, buffer, sizeof(buffer))) > 0) {
        // Append to request buffer
        size_t new_size = conn->request_size + bytes_read;
        if (new_size >= BUFFER_SIZE) {
            printf("Request too large\n");
            return -1;
        }
        
        memcpy(conn->request_buffer + conn->request_size, buffer, bytes_read);
        conn->request_size = new_size;
        conn->request_buffer[conn->request_size] = '\0';
        
        // Check if we have a complete HTTP request
        if (strstr(conn->request_buffer, "\r\n\r\n")) {
            // Parse the request
            if (http_parse_request(conn->request_buffer, &conn->http_request) < 0) {
                printf("Failed to parse HTTP request\n");
                return -1;
            }
            
            printf("HTTP Request: %s %s\n", conn->http_request.method, conn->http_request.path);
            
            // Check for special endpoints
            if (strcmp(conn->http_request.path, "/health") == 0) {
                return server_handle_health_endpoint(server, conn);
            } else if (strcmp(conn->http_request.path, "/stats") == 0) {
                return server_handle_stats_endpoint(server, conn);
            }
            
            // Select backend for load balancing
            uint32_t client_ip = ntohl(conn->client_addr.sin_addr.s_addr);
            backend_t *backend = lb_select_backend(server->lb, client_ip);
            
            if (!backend) {
                return server_send_error_response(conn, 503, "Service Unavailable");
            }
            
            conn->backend = backend;
            conn->state = CONN_CONNECTING_BACKEND;
            
            // Connect to backend
            return server_connect_to_backend(server, conn);
        }
    }
    
    if (bytes_read == -1) {
        if (errno != EAGAIN && errno != EWOULDBLOCK) {
            perror("read client");
            return -1;
        }
        // Would block, continue later
        return 0;
    }
    
    // Client closed connection
    return -1;
}

// Connect to backend server
static int server_connect_to_backend(server_t *server, connection_t *conn) {
    // Create socket to backend
    int backend_fd = socket(AF_INET, SOCK_STREAM, 0);
    if (backend_fd == -1) {
        perror("socket backend");
        return server_send_error_response(conn, 502, "Bad Gateway");
    }
    
    // Set non-blocking
    if (set_nonblocking(backend_fd) == -1) {
        close(backend_fd);
        return server_send_error_response(conn, 502, "Bad Gateway");
    }
    
    // Parse backend address
    struct sockaddr_in backend_addr;
    memset(&backend_addr, 0, sizeof(backend_addr));
    backend_addr.sin_family = AF_INET;
    backend_addr.sin_port = htons(conn->backend->port);
    
    if (inet_pton(AF_INET, conn->backend->host, &backend_addr.sin_addr) <= 0) {
        printf("Invalid backend address: %s\n", conn->backend->host);
        close(backend_fd);
        return server_send_error_response(conn, 502, "Bad Gateway");
    }
    
    // Connect to backend (non-blocking)
    int result = connect(backend_fd, (struct sockaddr*)&backend_addr, sizeof(backend_addr));
    if (result == -1 && errno != EINPROGRESS) {
        perror("connect backend");
        close(backend_fd);
        return server_send_error_response(conn, 502, "Bad Gateway");
    }
    
    conn->backend_fd = backend_fd;
    
    // Add backend socket to epoll for write notification
    struct epoll_event ev;
    ev.events = EPOLLOUT | EPOLLET;
    ev.data.ptr = conn;
    
    if (epoll_ctl(server->epoll_fd, EPOLL_CTL_ADD, backend_fd, &ev) == -1) {
        perror("epoll_ctl ADD backend");
        close(backend_fd);
        conn->backend_fd = -1;
        return server_send_error_response(conn, 502, "Bad Gateway");
    }
    
    printf("Connecting to backend %s:%d\n", conn->backend->host, conn->backend->port);
    return 0;
}

// Send error response to client
static int server_send_error_response(connection_t *conn, int status_code, const char *status_text) {
    char response[1024];
    int len = snprintf(response, sizeof(response),
        "HTTP/1.1 %d %s\r\n"
        "Content-Type: text/plain\r\n"
        "Content-Length: %zu\r\n"
        "Connection: close\r\n"
        "\r\n"
        "%s\n", 
        status_code, status_text, strlen(status_text) + 1, status_text);
    
    send(conn->client_fd, response, len, 0);
    return -1; // Signal connection should be closed
}

// Handle health check endpoint
static int server_handle_health_endpoint(server_t *server, connection_t *conn) {
    const char *response = 
        "HTTP/1.1 200 OK\r\n"
        "Content-Type: application/json\r\n"
        "Content-Length: 25\r\n"
        "\r\n"
        "{\"status\":\"healthy\"}\n";
    
    send(conn->client_fd, response, strlen(response), 0);
    return -1; // Close connection after response
}

// Handle stats endpoint
static int server_handle_stats_endpoint(server_t *server, connection_t *conn) {
    char *stats_json = stats_to_json(server->lb->stats);
    if (!stats_json) {
        return server_send_error_response(conn, 500, "Internal Server Error");
    }
    
    char response[4096];
    int len = snprintf(response, sizeof(response),
        "HTTP/1.1 200 OK\r\n"
        "Content-Type: application/json\r\n"
        "Content-Length: %zu\r\n"
        "\r\n"
        "%s", 
        strlen(stats_json), stats_json);
    
    send(conn->client_fd, response, len, 0);
    free(stats_json);
    
    return -1; // Close connection after response
}

// Cleanup connection resources
static void cleanup_connection(server_t *server, connection_t *conn) {
    if (conn->client_fd != -1) {
        epoll_ctl(server->epoll_fd, EPOLL_CTL_DEL, conn->client_fd, NULL);
        close(conn->client_fd);
        conn->client_fd = -1;
    }
    
    if (conn->backend_fd != -1) {
        epoll_ctl(server->epoll_fd, EPOLL_CTL_DEL, conn->backend_fd, NULL);
        close(conn->backend_fd);
        conn->backend_fd = -1;
    }
    
    free(conn->request_buffer);
    free(conn->response_buffer);
    conn->request_buffer = NULL;
    conn->response_buffer = NULL;
    
    server->active_connections--;
}

// Initialize server
int server_init(server_t *server, loadbalancer_t *lb) {
    memset(server, 0, sizeof(server_t));
    
    server->lb = lb;
    server->running = 1;
    server->max_connections = lb->max_connections;
    
    // Create listening socket
    server->listen_fd = create_listen_socket(lb->server_port);
    if (server->listen_fd == -1) {
        return -1;
    }
    
    // Create epoll instance
    server->epoll_fd = epoll_create1(EPOLL_CLOEXEC);
    if (server->epoll_fd == -1) {
        perror("epoll_create1");
        close(server->listen_fd);
        return -1;
    }
    
    // Add listening socket to epoll
    struct epoll_event ev;
    ev.events = EPOLLIN;
    ev.data.fd = server->listen_fd;
    
    if (epoll_ctl(server->epoll_fd, EPOLL_CTL_ADD, server->listen_fd, &ev) == -1) {
        perror("epoll_ctl listening socket");
        close(server->listen_fd);
        close(server->epoll_fd);
        return -1;
    }
    
    // Allocate connection array
    server->connections = calloc(server->max_connections, sizeof(connection_t));
    if (!server->connections) {
        perror("calloc connections");
        close(server->listen_fd);
        close(server->epoll_fd);
        return -1;
    }
    
    // Initialize all connections as unused
    for (int i = 0; i < server->max_connections; i++) {
        server->connections[i].client_fd = -1;
        server->connections[i].backend_fd = -1;
    }
    
    printf("Server initialized on port %d (max connections: %d)\n", 
           lb->server_port, server->max_connections);
    
    return 0;
}

// Main server event loop
int server_run(server_t *server) {
    struct epoll_event events[MAX_EVENTS];
    
    printf("Server starting event loop...\n");
    
    while (server->running) {
        int nfds = epoll_wait(server->epoll_fd, events, MAX_EVENTS, 1000); // 1 second timeout
        
        if (nfds == -1) {
            if (errno == EINTR) {
                continue; // Interrupted by signal, continue
            }
            perror("epoll_wait");
            return -1;
        }
        
        for (int i = 0; i < nfds; i++) {
            if (events[i].data.fd == server->listen_fd) {
                // New client connection
                handle_accept(server);
            } else {
                // Existing connection event
                connection_t *conn = (connection_t*)events[i].data.ptr;
                
                if (events[i].events & (EPOLLERR | EPOLLHUP)) {
                    printf("Connection error/hangup\n");
                    cleanup_connection(server, conn);
                    continue;
                }
                
                if (events[i].events & EPOLLIN) {
                    // Data ready to read
                    if (handle_client_read(server, conn) < 0) {
                        cleanup_connection(server, conn);
                    }
                }
                
                if (events[i].events & EPOLLOUT) {
                    // Ready to write (backend connection established)
                    // TODO: Implement backend communication
                    printf("Backend connection ready for writing\n");
                }
            }
        }
    }
    
    printf("Server event loop ended\n");
    return 0;
}

// Cleanup server resources
void server_cleanup(server_t *server) {
    printf("Cleaning up server...\n");
    
    // Close all connections
    if (server->connections) {
        for (int i = 0; i < server->max_connections; i++) {
            if (server->connections[i].client_fd != -1) {
                cleanup_connection(server, &server->connections[i]);
            }
        }
        free(server->connections);
    }
    
    // Close epoll and listening socket
    if (server->epoll_fd != -1) {
        close(server->epoll_fd);
    }
    
    if (server->listen_fd != -1) {
        close(server->listen_fd);
    }
    
    printf("Server cleanup complete\n");
}