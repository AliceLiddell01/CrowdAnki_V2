// Пакет main предоставляет точку входа для нативного ядра CrowdAnki V2.
// Ядро реализует основную вычислительную логику аддона на языке Go.
package main

import (
	"fmt"
	"runtime"
)

// appVersion определяет текущую версию исполняемого файла ядра.
const appVersion = "0.1.0"

func main() {
	fmt.Printf("CrowdAnki V2 Core v%s (%s/%s)\n", appVersion, runtime.GOOS, runtime.GOARCH)
}
