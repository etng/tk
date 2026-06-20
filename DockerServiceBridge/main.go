package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"
)

const defaultAddr = "127.0.0.1:17321"
const allowContainerBindEnv = "TOOLDKIT_ALLOW_CONTAINER_BIND"

type containerInfo struct {
	ID     string
	Names  []string
	Labels map[string]string
}

type serviceLink struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
	Group       string `json:"group,omitempty"`
	Icon        string `json:"icon,omitempty"`
}

type serviceFeed struct {
	GeneratedAt string        `json:"generatedAt"`
	Links       []serviceLink `json:"links"`
}

type serviceConfig struct {
	AllowedOrigins map[string]struct{}
	Token          string
	ListContainers func() ([]containerInfo, error)
}

type dockerInspectContainer struct {
	ID     string `json:"Id"`
	Name   string `json:"Name"`
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"Config"`
}

func main() {
	addr := flag.String("addr", defaultAddr, "HTTP listen address. Must be localhost or loopback unless container bind is explicitly enabled.")
	origins := flag.String("origins", "https://etng.github.io,http://127.0.0.1:5173,http://localhost:5173", "Comma separated allowed browser origins.")
	token := flag.String("token", "", "Optional request token. If set, clients must send X-Tooldkit-Token.")
	flag.Parse()

	if err := validateListenAddress(*addr, allowContainerBind()); err != nil {
		log.Fatal(err)
	}

	handler := newServiceHandler(serviceConfig{
		AllowedOrigins: parseAllowedOrigins(*origins),
		Token:          strings.TrimSpace(*token),
		ListContainers: listDockerContainers,
	})
	server := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 3 * time.Second,
	}
	log.Printf("Docker service bridge listening on http://%s", *addr)
	log.Fatal(server.ListenAndServe())
}

func newServiceHandler(config serviceConfig) http.Handler {
	if config.ListContainers == nil {
		config.ListContainers = listDockerContainers
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(response http.ResponseWriter, request *http.Request) {
		if !allowRequest(response, request, config) {
			return
		}
		if request.Method == http.MethodOptions {
			response.WriteHeader(http.StatusNoContent)
			return
		}
		if request.Method != http.MethodGet {
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		response.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = response.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/v1/services", func(response http.ResponseWriter, request *http.Request) {
		if !allowRequest(response, request, config) {
			return
		}
		if request.Method == http.MethodOptions {
			response.WriteHeader(http.StatusNoContent)
			return
		}
		if request.Method != http.MethodGet {
			http.Error(response, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		containers, err := config.ListContainers()
		if err != nil {
			http.Error(response, err.Error(), http.StatusBadGateway)
			return
		}
		links := buildLinks(containers)
		response.Header().Set("Content-Type", "application/json; charset=utf-8")
		_ = json.NewEncoder(response).Encode(serviceFeed{
			GeneratedAt: time.Now().UTC().Format(time.RFC3339),
			Links:       links,
		})
	})
	return mux
}

func allowRequest(response http.ResponseWriter, request *http.Request, config serviceConfig) bool {
	origin := request.Header.Get("Origin")
	if origin != "" {
		if _, ok := config.AllowedOrigins[origin]; !ok {
			http.Error(response, "origin not allowed", http.StatusForbidden)
			return false
		}
		response.Header().Set("Access-Control-Allow-Origin", origin)
		response.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		response.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Tooldkit-Token")
		response.Header().Set("Access-Control-Allow-Private-Network", "true")
		response.Header().Set("Vary", "Origin")
	}
	if config.Token != "" &&
		request.Method != http.MethodOptions &&
		request.Header.Get("X-Tooldkit-Token") != config.Token &&
		request.URL.Query().Get("token") != config.Token {
		http.Error(response, "token required", http.StatusUnauthorized)
		return false
	}
	return true
}

func buildLinks(containers []containerInfo) []serviceLink {
	links := make([]serviceLink, 0, len(containers))
	for _, container := range containers {
		link, ok := buildLinkFromContainer(container)
		if ok {
			links = append(links, link)
		}
	}
	sort.SliceStable(links, func(i, j int) bool {
		if links[i].Group != links[j].Group {
			return links[i].Group < links[j].Group
		}
		return links[i].Title < links[j].Title
	})
	return links
}

func buildLinkFromContainer(container containerInfo) (serviceLink, bool) {
	labels := container.Labels
	if strings.EqualFold(labels["homepage.enabled"], "false") {
		return serviceLink{}, false
	}
	url := firstNonBlank(labels["homepage.href"], labels["homepage.url"])
	if url == "" {
		return serviceLink{}, false
	}
	title := firstNonBlank(labels["homepage.name"], labels["homepage.title"], containerName(container))
	return serviceLink{
		ID:          "docker:" + shortContainerID(container.ID),
		Title:       title,
		URL:         url,
		Description: strings.TrimSpace(labels["homepage.description"]),
		Group:       firstNonBlank(labels["homepage.group"], "Docker"),
		Icon:        strings.TrimSpace(labels["homepage.icon"]),
	}, true
}

func listDockerContainers() ([]containerInfo, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	listOutput, err := exec.CommandContext(ctx, "docker", "container", "ls", "-q").Output()
	if err != nil {
		return nil, err
	}
	ids := strings.Fields(string(listOutput))
	if len(ids) == 0 {
		return []containerInfo{}, nil
	}

	args := append([]string{"inspect"}, ids...)
	inspectOutput, err := exec.CommandContext(ctx, "docker", args...).Output()
	if err != nil {
		return nil, err
	}
	var inspected []dockerInspectContainer
	if err := json.Unmarshal(inspectOutput, &inspected); err != nil {
		return nil, err
	}
	containers := make([]containerInfo, 0, len(inspected))
	for _, item := range inspected {
		containers = append(containers, containerInfo{
			ID:     item.ID,
			Names:  []string{item.Name},
			Labels: item.Config.Labels,
		})
	}
	return containers, nil
}

func validateListenAddress(addr string, allowContainerBind bool) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if allowContainerBind && (host == "" || host == "0.0.0.0" || host == "::") {
		return nil
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return errors.New("listen address must be localhost or a loopback IP")
	}
	return nil
}

func allowContainerBind() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(allowContainerBindEnv))) {
	case "1", "true", "yes":
		return true
	default:
		return false
	}
}

func parseAllowedOrigins(value string) map[string]struct{} {
	origins := map[string]struct{}{}
	for _, part := range strings.Split(value, ",") {
		origin := strings.TrimSpace(part)
		if origin != "" {
			origins[origin] = struct{}{}
		}
	}
	return origins
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if clean := strings.TrimSpace(value); clean != "" {
			return clean
		}
	}
	return ""
}

func containerName(container containerInfo) string {
	for _, name := range container.Names {
		if clean := strings.Trim(strings.TrimSpace(name), "/"); clean != "" {
			return clean
		}
	}
	return shortContainerID(container.ID)
}

func shortContainerID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}
