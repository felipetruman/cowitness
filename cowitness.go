package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	"github.com/miekg/dns"
)

const (
	HTTPPort  = 80
	HTTPSPort = 443
	DNSPort   = 53
)

var (
	DNSResponseIP   string
	DNSResponseName string
	DefaultTTL      int
)

func main() {
	displayBanner()

	rootDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	requestUserInputs()

	httpLogFile, dnsLogFile := createLogFiles()
	defer closeLogFiles(httpLogFile, dnsLogFile)

	// Create HTTP request logger
	httpLogger := log.New(httpLogFile, "", log.LstdFlags)

	startHTTPServer(HTTPPort, rootDir, httpLogger)
	startHTTPServer(HTTPSPort, rootDir, httpLogger)
	startDNSServer(DNSPort, dnsLogFile)

	log.Printf("Open the following URL in your browser:\n")
	log.Printf("http://localhost:%d\n", HTTPPort)

	// Create a channel to receive OS signals
	c := make(chan os.Signal, 1)
	// Notify the channel for given signals
	signal.Notify(c, os.Interrupt)

	// Use a goroutine to keep the main function executing and
	// listen to the OS signals.
	// If an interrupt or kill signal comes,
	// cleanup resources by calling killDNSonExit()
	go func() {
		<-c
		// cleanup and exit
		killDNSonExit()
		os.Exit(0)
	}()

	// Wait indefinitely
	select {}
}

func requestUserInputs() {
	fmt.Print("Enter the DNS response IP: ")
	fmt.Scanln(&DNSResponseIP)

	fmt.Print("Enter the DNS response name: ")
	fmt.Scanln(&DNSResponseName)

	fmt.Print("Enter the Default TTL: ")
	fmt.Scanln(&DefaultTTL)
}

func createLogFiles() (*os.File, *os.File) {
	httpLogFile, err := os.OpenFile("./http.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}

	dnsLogFile, err := os.OpenFile("./dns.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Fatal(err)
	}

	return httpLogFile, dnsLogFile
}

func closeLogFiles(httpLogFile, dnsLogFile *os.File) {
	httpLogFile.Close()
	dnsLogFile.Close()
}

func startHTTPServer(port int, rootDir string, httpLogger *log.Logger) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ipAddress := strings.Split(r.RemoteAddr, ":")[0]
		requestResource := r.URL.Path
		userAgent := r.UserAgent()
		logMessage := fmt.Sprintf("IP address: %s, Resource: %s, User agent: %s\n", ipAddress, requestResource, userAgent)
		httpLogger.Println(logMessage)

		http.FileServer(http.Dir(rootDir)).ServeHTTP(w, r)
	})

	go func() {
		log.Printf("Starting HTTP server on port %d\n", port)
		err := http.ListenAndServe(fmt.Sprintf(":%d", port), mux)
		if err != nil {
			log.Fatal(err)
		}
	}()
}

func startDNSServer(port int, dnsLogFile *os.File) {
	addr := fmt.Sprintf(":%d", port)
	server := &dns.Server{Addr: addr, Net: "udp"}

	dns.HandleFunc(".", func(w dns.ResponseWriter, r *dns.Msg) {
		handleDNSQuery(w, r, dnsLogFile)
	})

	go func() {
		log.Printf("Starting DNS server on port %d\n", port)
		err := server.ListenAndServe()
		if err != nil {
			log.Fatal(err)
		}
	}()
}

func handleDNSQuery(w dns.ResponseWriter, r *dns.Msg, dnsLogFile *os.File) {
	ipAddress := w.RemoteAddr().(*net.UDPAddr).IP
	logMessage := fmt.Sprintf("IP address: %s, DNS request: %s\n", ipAddress, r.Question[0].Name)
	if _, err := dnsLogFile.WriteString(logMessage); err != nil {
		log.Println(err)
	}

	response := new(dns.Msg)
	response.SetReply(r)
	response.Authoritative = true
	response.RecursionAvailable = true

	domain := r.Question[0].Name
	subdomain := strings.TrimSuffix(domain, "."+DNSResponseName)

	if r.Question[0].Qtype == dns.TypeNS {
		response.Answer = append(response.Answer,
			&dns.NS{
				Hdr: dns.RR_Header{Name: DNSResponseName, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: uint32(DefaultTTL)},
				Ns:  "ns1.domain.com.",
			},
			&dns.NS{
				Hdr: dns.RR_Header{Name: DNSResponseName, Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: uint32(DefaultTTL)},
				Ns:  "ns2.domain.com.",
			})
	} else if r.Question[0].Qtype == dns.TypeA {
		if domain == DNSResponseName {
			response.Answer = append(response.Answer,
				&dns.A{
					Hdr: dns.RR_Header{Name: DNSResponseName, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: uint32(DefaultTTL)},
					A:   net.ParseIP(DNSResponseIP),
				})
		} else {
			response.Answer = append(response.Answer,
				&dns.A{
					Hdr: dns.RR_Header{Name: subdomain + "." + DNSResponseName, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: uint32(DefaultTTL)},
					A:   net.ParseIP(DNSResponseIP),
				})
		}
	}

	if err := w.WriteMsg(response); err != nil {
		log.Println(err)
	}
}

func killDNSonExit() {
	defer func() {
		pid := os.Getpid()
		cmd := exec.Command("kill", "-9", fmt.Sprintf("%d", pid))
		err := cmd.Run()
		if err != nil {
			log.Println(err)
		}
	}()
}

func displayBanner() {
	red := "\033[31m"
	reset := "\033[0m"
	cowitnessVersion := "v1.1"
	banner := red + `
 	          ⢠⡄
	    	⣠⣤⣾⣷⣤⣄⡀⠀⠀⠀⠀
  @@@@@@     ⣴⡿⠋⠁⣼⡇⠈⠙⢿⣧⠀⠀⠀ @@@  @@@  @@@  @@@  @@@@@@@ @@@  @@@ @@@@@@@@  @@@@@@  @@@@@@
 !@@        ⣸⡟⠀⠀⠀⠘⠃⠀⠀⠀⢻⣇⠀⠀ @@!  @@!  @@!  @@!    @@!   @@!@!@@@ @@!      !@@     !@@    
 !@!     ⠰⠶⣿⡷⠶⠶⠀⠀⠀⠀⠶⠶⢾⣿⠶⠆  @!!  !!@  @!@  !!@    @!!   @!@@!!@! @!!!:!    !@@!!   !@@!! 
 :!!        ⢹⣧⠀⠀⠀⢠⡄⠀⠀⠀⣼⡏⠀   !:  !!:  !!   !!:    !!:   !!:  !!! !!:          !:!     !:!
  :: :: :    ⠹⣷⣆⡀⢸⡇⢀⣠⣾⠏⠀⠀⠀⠀  ::.:  :::    :       :    ::    :  : :: ::: ::.: :  ::.: :
               ⠈⠙⠛⣿⡿⠛⠋⠀⠀⠀⠀  
	           ⠘⠃⠀⠀
` + reset

	fmt.Print(banner)
	fmt.Println("             CoWitness", cowitnessVersion, "- Tool for HTTP, HTTPS, and DNS Server")
	fmt.Println()
}
