package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	TargetAddr = "127.0.0.1"
)

const (
	ServerAddr       = "0.0.0.0"
	ServerPort       = "8080"
	TargetPortSSH    = "22"
	TargetPortV2Ray  = "8080"
	BufferSize       = 524288           // 512 KB
	KeepAliveTimeout = 24 * time.Hour   // Mantém conexão viva por até 24 horas
)

type Target struct {
	Addr  string
	Port  string
	V2Ray bool
}

func createTarget(endpoint string) *Target {
	if endpoint == "/ws/" {
		return &Target{Addr: TargetAddr, Port: TargetPortV2Ray, V2Ray: true}
	}
	return &Target{Addr: TargetAddr, Port: TargetPortSSH, V2Ray: false}
}

func copyStream(src, dst net.Conn, wg *sync.WaitGroup, direction string) {
	defer wg.Done()
	buffer := make([]byte, BufferSize)
	for {
		n, err := src.Read(buffer)
		if err != nil {
			if err != io.EOF {
				fmt.Printf("[ERROR] Erro ao transferir dados (%s): %v\n", direction, err)
			}
			break
		}
		if n > 0 {
			_, err := dst.Write(buffer[:n])
			if err != nil {
				fmt.Printf("[ERROR] Erro ao escrever dados (%s): %v\n", direction, err)
				break
			}
		}
	}
}

func keepAlive(conns ...net.Conn) {
	ticker := time.NewTicker(KeepAliveTimeout)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			for _, conn := range conns {
				if err := conn.SetDeadline(time.Now().Add(KeepAliveTimeout)); err != nil {
					fmt.Printf("[WARN] Conexão encerrada durante Keep-Alive: %v\n", err)
					return
				}
			}
		}
	}
}

func handleClient(client net.Conn) {
	defer client.Close()

	clientAddr := client.RemoteAddr().String()
	fmt.Printf("[INFO] Cliente conectado: %s\n", clientAddr)

	buffer := make([]byte, BufferSize)
	size, err := client.Read(buffer)
	if err != nil {
		fmt.Printf("[ERROR] Falha ao ler do cliente (%s): %v\n", clientAddr, err)
		return
	}

	payload := string(buffer[:size])
	fmt.Printf("[DEBUG] Payload recebido: %s\n", strings.ReplaceAll(payload, "\n", "\\n"))

	endpoint := strings.Split(payload, " ")[1]
	target := createTarget(endpoint)

	targetConn, err := net.Dial("tcp", net.JoinHostPort(target.Addr, target.Port))
	if err != nil {
		fmt.Printf("[ERROR] Falha ao conectar no alvo (%s:%s): %v\n", target.Addr, target.Port, err)
		return
	}
	defer targetConn.Close()

	if target.V2Ray {
		targetConn.Write(buffer[:size])
	} else {
		client.Write([]byte("HTTP/1.1 101 Switching Protocols\r\nUpgrade: Websocket\r\nConnection: Upgrade\r\n\r\n"))
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go copyStream(client, targetConn, &wg, "Cliente -> Alvo")
	go copyStream(targetConn, client, &wg, "Alvo -> Cliente")

	go keepAlive(client, targetConn)

	wg.Wait()
	fmt.Printf("[INFO] Conexão encerrada: %s\n", clientAddr)
}

func main() {
	serverAddr := net.JoinHostPort(ServerAddr, ServerPort)
	listener, err := net.Listen("tcp", serverAddr)
	if err != nil {
		fmt.Printf("[FATAL] Falha ao iniciar o servidor: %v\n", err)
		os.Exit(1)
	}
	defer listener.Close()

	fmt.Printf("[INFO] Servidor escutando em %s\n IP:%s\n", serverAddr, TargetAddr)

	for {
		client, err := listener.Accept()
		if err != nil {
			fmt.Printf("[ERROR] Falha ao aceitar conexão: %v\n", err)
			continue
		}

		go handleClient(client)
	}
}
