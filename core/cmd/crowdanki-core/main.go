package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/AliceLiddell01/CrowdAnki_V2/core/pkg/export"
)

// appVersion определяет текущую версию исполняемого файла ядра.
const appVersion = "0.1.0"

func main() {
	if len(os.Args) > 1 && os.Args[1] == "export" {
		runExport()
		return
	}

	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("CrowdAnki V2 Core v%s (%s/%s)\n", appVersion, runtime.GOOS, runtime.GOARCH)
		return
	}

	fmt.Printf("CrowdAnki V2 Core v%s (%s/%s)\n", appVersion, runtime.GOOS, runtime.GOARCH)
	fmt.Println("Использование: crowdanki-core <команда>")
	fmt.Println("Команды:")
	fmt.Println("  export   Выполнить экспорт коллекции (входные данные через stdin в формате JSON)")
	fmt.Println("  version  Вывести информацию о версии")
}

func runExport() {
	inputData, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка чтения входных данных из stdin: %v\n", err)
		os.Exit(1)
	}

	var req export.ExportRequest
	if err := json.Unmarshal(inputData, &req); err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка декодирования JSON входных данных: %v\n", err)
		os.Exit(2)
	}

	result, err := export.ExecuteExport(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка при выполнении экспорта: %v\n", err)
		os.Exit(3)
	}

	outputBytes, err := json.Marshal(result)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Ошибка кодирования результата экспорта: %v\n", err)
		os.Exit(4)
	}

	os.Stdout.Write(outputBytes)
	os.Stdout.WriteString("\n")
}
