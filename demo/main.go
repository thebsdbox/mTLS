package main

import (
	"fmt"
	"net"
	"os"
	"time"
)

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
	defer listen.Close()

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
				fmt.Println(err)
				conn.Close()
				break
			}
			fmt.Printf("%s from %s\n", buffer, conn.RemoteAddr().String())
		}
	}
}

func connector(client, hostname string) {
	address := fmt.Sprintf("%s:9000", client)
	fmt.Printf("connecting to %s\n", client)
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
