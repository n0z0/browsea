package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
)

// CookieData struct untuk menyimpan data cookie untuk simulasi
type CookieData struct {
	Name     string  `json:"name"`
	Value    string  `json:"value"`
	Domain   string  `json:"domain"`
	Path     string  `json:"path"`
	HttpOnly bool    `json:"httpOnly"`
	Secure   bool    `json:"secure"`
	Expires  float64 `json:"expires"`
	Session  bool    `json:"session"`
}

func main() {
	path, ok := launcher.LookPath()
	if !ok {
		path = "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe"
	}

	u, err := launcher.New().
		Bin(path).
		Headless(false).
		Leakless(false). // Mencegah pembuatan binary leakless di Temp yang dianggap malware
		Launch()

	if err != nil {
		fmt.Printf("\n[!] Gagal menjalankan browser!\nError: %v\n", err)
		fmt.Println("\nJika Anda melihat error 'virus or potentially unwanted software', ini adalah efek dari Windows Defender/Antivirus yang memblokir simulasi session hijacking kita.")
		fmt.Println("Solusinya: Tambahkan folder project ini (atau file executable-nya) ke dalam daftar Exclusion/Pengecualian di Windows Security Anda.")
		return
	}

	// Inisialisasi browser
	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	fmt.Println("Browser terbuka. Silakan interaksi / login ke aplikasi web target...")
	fmt.Println("Script ini akan memantau cookie untuk simulasi session hijacking.")
	fmt.Println("Tekan Ctrl+C untuk berhenti monitoring.")

	// Navigasi ke target
	// Ganti dengan URL aplikasi yang ingin ditest
	url := "https://example.com" // Contoh default, bisa diganti
	page := browser.MustPage(url)

	page.MustWaitLoad()

	// Monitor cookies secara berkala
	monitorCookies(browser)
}

func monitorCookies(browser *rod.Browser) {
	ticker := time.NewTicker(3 * time.Second) // Cek setiap 3 detik
	defer ticker.Stop()

	var previousCookiesStr string

	for range ticker.C {
		// Dapatkan semua halaman
		pages, err := browser.Pages()
		if err != nil || len(pages) == 0 {
			continue
		}

		// Kita monitor cookie di halaman aktif yang pertama
		page := pages[0]
		
		// Mengambil semua cookies untuk URL yang ada di halaman tersebut
		cookies, err := page.Cookies([]string{})
		if err != nil {
			log.Printf("Error mendapatkan cookies: %v", err)
			continue
		}

		if len(cookies) > 0 {
			currentCookiesJSON, _ := json.Marshal(cookies)
			currentCookiesStr := string(currentCookiesJSON)

			// Jika ada perubahan cookie
			if currentCookiesStr != previousCookiesStr {
				fmt.Println("\n[!] Cookie Baru / Perubahan Session Terdeteksi!")
				
				var cookieDataList []CookieData
				for _, c := range cookies {
					cookieDataList = append(cookieDataList, CookieData{
						Name:     c.Name,
						Value:    c.Value,
						Domain:   c.Domain,
						Path:     c.Path,
						HttpOnly: c.HTTPOnly,
						Secure:   c.Secure,
						Expires:  float64(c.Expires),
						Session:  c.Session,
					})
					
					// Highlight cookie yang rentan atau menarik
					warning := ""
					if !c.HTTPOnly {
						warning += " [HTTPOnly=false]"
					}
					if !c.Secure {
						warning += " [Secure=false]"
					}
					
					// Jangan print value terlalu panjang
					fmt.Printf("- %s = %s... %s\n", c.Name, truncateString(c.Value, 40), warning)

					nameLower := strings.ToLower(c.Name)
					if strings.Contains(nameLower, "sess") || strings.Contains(nameLower, "auth") || strings.Contains(nameLower, "token") || strings.Contains(nameLower, "id") {
						fmt.Printf("   >>> MUNGKIN TARGET: Session / Token: %s\n", c.Name)
						fmt.Printf("   >>> PAYLOAD HIJACK: document.cookie=\"%s=%s; domain=%s; path=%s\";\n", c.Name, c.Value, c.Domain, c.Path)
					}
				}

				// Simpan ke file log JSON
				saveToJSON(cookieDataList, "hijacked_cookies.json")
				
				previousCookiesStr = currentCookiesStr
			}
		}
	}
}

func truncateString(str string, length int) string {
	if len(str) <= length {
		return str
	}
	return str[:length]
}

func saveToJSON(data interface{}, filename string) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	fmt.Printf("\n[+] Data tersimpan ke %s\n", filename)
	return os.WriteFile(filename, jsonData, 0644)
}
