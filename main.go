package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/chromedp/chromedp"
)

// FormData struct untuk menyimpan data yang dikumpulkan
type FormData struct {
	Name      string    `json:"name"`
	Email     string    `json:"email"`
	Phone     string    `json:"phone"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

func main() {
	// Buat context dengan browser visible (bukan headless)
	// Agar user bisa melihat dan mengisi form
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false), // PENTING: false agar browser terlihat
		chromedp.Flag("disable-gpu", false),
		chromedp.Flag("no-sandbox", true),
		chromedp.WindowSize(1280, 720),
	)

	allocCtx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()

	ctx, cancel := chromedp.NewContext(allocCtx, chromedp.WithLogf(log.Printf))
	defer cancel()

	// Navigasi ke halaman form
	url := "https://www.example.com/contact" // Ganti dengan URL form Anda yang sesungguhnya

	err := chromedp.Run(ctx,
		chromedp.Navigate(url),
		chromedp.WaitVisible(`body`, chromedp.ByQuery),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Browser terbuka. Silakan isi form secara manual...")
	fmt.Println("Tekan Ctrl+C untuk berhenti monitoring")

	// Monitor form secara berkala
	monitorFormInput(ctx)
}

// monitorFormInput memantau input user secara real-time
func monitorFormInput(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second) // Cek setiap 2 detik
	defer ticker.Stop()

	previousData := FormData{}

	for {
		select {
		case <-ticker.C:
			// Ambil data form saat ini
			currentData, err := collectFormData(ctx)
			if err != nil {
				log.Printf("Error collecting data: %v", err)
				continue
			}

			// Cek apakah ada perubahan
			if hasChanged(previousData, *currentData) {
				fmt.Println("\n=== Data Form Berubah ===")
				printFormData(currentData)

				// Simpan ke database atau file
				saveToJSON(currentData, "form_data.json")

				previousData = *currentData
			}

		case <-ctx.Done():
			return
		}
	}
}

// collectFormData mengumpulkan data dari form yang diisi user
func collectFormData(ctx context.Context) (*FormData, error) {
	var data FormData
	data.Timestamp = time.Now()

	// Ambil nilai dari berbagai tipe input
	err := chromedp.Run(ctx,
		// Text inputs
		chromedp.Value(`input[name="name"]`, &data.Name, chromedp.ByQuery),
		chromedp.Value(`input[name="email"]`, &data.Email, chromedp.ByQuery),
		chromedp.Value(`input[name="phone"]`, &data.Phone, chromedp.ByQuery),

		// Textarea
		chromedp.Value(`textarea[name="message"]`, &data.Message, chromedp.ByQuery),
	)

	if err != nil {
		return nil, err
	}

	return &data, nil
}

// collectFormDataDynamic mengumpulkan semua input secara dinamis
func collectFormDataDynamic(ctx context.Context, formSelector string) (map[string]interface{}, error) {
	var result []map[string]interface{}

	script := fmt.Sprintf(`
		(function() {
			const form = document.querySelector('%s');
			if (!form) return [];
			
			const data = [];
			const inputs = form.querySelectorAll('input, textarea, select');
			
			inputs.forEach(input => {
				let value = input.value;
				
				// Handle checkbox dan radio
				if (input.type === 'checkbox' || input.type === 'radio') {
					value = input.checked;
				}
				
				// Handle select multiple
				if (input.type === 'select-multiple') {
					value = Array.from(input.selectedOptions).map(o => o.value);
				}
				
				data.push({
					name: input.name || input.id,
					type: input.type,
					value: value,
					tagName: input.tagName.toLowerCase()
				});
			});
			
			return data;
		})();
	`, formSelector)

	err := chromedp.Run(ctx,
		chromedp.Evaluate(script, &result),
	)

	if err != nil {
		return nil, err
	}

	// Convert to map
	formData := make(map[string]interface{})
	for _, field := range result {
		if name, ok := field["name"].(string); ok && name != "" {
			formData[name] = field["value"]
		}
	}

	return formData, nil
}

// waitForSubmit menunggu user submit form
func waitForSubmit(ctx context.Context, submitButtonSelector string) (*FormData, error) {
	fmt.Println("Menunggu user submit form...")

	// Tunggu tombol submit diklik
	err := chromedp.Run(ctx,
		chromedp.WaitVisible(submitButtonSelector, chromedp.ByQuery),
	)
	if err != nil {
		return nil, err
	}

	// Setelah submit, ambil data
	data, err := collectFormData(ctx)
	if err != nil {
		return nil, err
	}

	fmt.Println("\n=== Form Submitted! ===")
	return data, nil
}

// listenToSubmitEvent mendengarkan event submit form
func listenToSubmitEvent(ctx context.Context) (*FormData, error) {
	// Inject script untuk capture submit event
	script := `
		(function() {
			return new Promise((resolve) => {
				const form = document.querySelector('form');
				if (form) {
					form.addEventListener('submit', function(e) {
						// e.preventDefault(); // Uncomment jika tidak ingin form benar-benar submit
						
						const formData = new FormData(form);
						const data = {};
						formData.forEach((value, key) => {
							data[key] = value;
						});
						resolve(data);
					});
				}
			});
		})();
	`

	var result map[string]interface{}
	err := chromedp.Run(ctx,
		chromedp.Evaluate(script, &result),
	)

	if err != nil {
		return nil, err
	}

	// Convert to FormData struct
	data := &FormData{
		Name:      getStringValue(result, "name"),
		Email:     getStringValue(result, "email"),
		Phone:     getStringValue(result, "phone"),
		Message:   getStringValue(result, "message"),
		Timestamp: time.Now(),
	}

	return data, nil
}

// Helper functions

func hasChanged(old, new FormData) bool {
	return old.Name != new.Name ||
		old.Email != new.Email ||
		old.Phone != new.Phone ||
		old.Message != new.Message
}

func printFormData(data *FormData) {
	fmt.Printf("Name    : %s\n", data.Name)
	fmt.Printf("Email   : %s\n", data.Email)
	fmt.Printf("Phone   : %s\n", data.Phone)
	fmt.Printf("Message : %s\n", data.Message)
	fmt.Printf("Time    : %s\n", data.Timestamp.Format("2006-01-02 15:04:05"))
}

func saveToJSON(data *FormData, filename string) error {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	fmt.Printf("\nData saved to %s\n", filename)
	fmt.Printf("%s\n", string(jsonData))

	// Simpan ke file
	return os.WriteFile(filename, jsonData, 0644)
}

func getStringValue(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// Contoh penggunaan alternatif:

// Example 1: Wait untuk user selesai mengisi (detect submit button click)
func exampleWaitForUserComplete(ctx context.Context) {
	url := "https://example.com/form"

	chromedp.Run(ctx,
		chromedp.Navigate(url),
	)

	fmt.Println("Silakan isi form...")

	// Tunggu hingga tombol submit diklik
	var submitted bool
	chromedp.Run(ctx,
		chromedp.Click(`button[type="submit"]`, chromedp.ByQuery),
		chromedp.Sleep(1*time.Second),
	)

	if submitted {
		data, _ := collectFormData(ctx)
		printFormData(data)
	}
}

// Example 2: Monitor secara periodik dan simpan ke database
func examplePeriodicSave(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		data, err := collectFormData(ctx)
		if err != nil {
			continue
		}

		// Simpan ke database
		// db.Save(data)
		fmt.Println("Data tersimpan:", data.Name)
	}
}
