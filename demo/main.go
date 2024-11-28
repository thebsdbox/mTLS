package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"time"
)

func helloHandler(writer http.ResponseWriter, r *http.Request) {
	fmt.Printf("GET from %s\n", r.RemoteAddr)
	r.Body.Close()
	//writer.Write([]byte("OK"))
}

func main() {
	client := os.Getenv("CLIENT")
	if client == "" {
		panic("No Client address to connect to")
	}
	hostname, err := os.Hostname()
	if err != nil {
		panic(err)
	}
	fmt.Printf("I am %s\n", hostname)

	go connector(client, hostname)

	listen, err := net.Listen("tcp", ":9000")
	if err != nil {
		panic(err)
	}
	go curler(client)

	defer listen.Close()
	http.HandleFunc("/hello", helloHandler)

	// Set the HTTP listener in a seperate go function as it is blocking
	go func() {
		err = http.ListenAndServe(":9080", nil)
		if err != nil {
			panic(err)
		}
	}()
	for {
		conn, err := listen.Accept()
		if err != nil {
			fmt.Println(err)
			continue
		}
		for {
			buffer := make([]byte, 256)
			_, err = conn.Read(buffer)
			if err != nil {
				fmt.Printf("[TCP session error] %v", err)
				conn.Close()
				break
			}
			fmt.Printf("%s from %s\n", buffer, conn.RemoteAddr().String())
		}
	}
}

func connector(client, hostname string) {
	address := fmt.Sprintf("%s:9000", client)
	fmt.Printf("[TCP session] connecting to %s\n", client)
	for {
		conn, err := net.DialTimeout("tcp", address, time.Second*3)
		if err != nil {
			fmt.Printf("unable to connect to %s, will try in 5 seconds, %v\n", client, err)
			time.Sleep(time.Second * 5)
			continue
		} else {
			for {
				_, err = conn.Write([]byte(fmt.Sprintf("Hello from %s", hostname)))
				if err != nil {
					conn.Close()
					break
				}
				time.Sleep(time.Second * 5)
			}
		}

	}
}

func curler(client string) {

	address := fmt.Sprintf("http://%s:9080/hello", client)
	fmt.Printf("[HTTP] connecting to %s\n", client)
	for {
		// New client for each connection
		c := http.Client{}
		c.CloseIdleConnections()
		r, err := c.Get(address)
		if err != nil {
			fmt.Printf("unable to connect to %s, will try in 5 seconds, %v\n", client, err)
			time.Sleep(time.Second * 5)
			continue
		}
		r.Body.Close()
		time.Sleep(time.Second * 5)
	}
}
