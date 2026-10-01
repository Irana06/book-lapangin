// Command manage berisi perintah administrasi platform:
//
//	go run ./cmd/manage seed           isi data demo untuk development
//	go run ./cmd/manage create-venue   daftarkan venue (klien) baru beserta akun pemiliknya
package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	_ "time/tzdata"

	"golang.org/x/term"

	"lapangin/internal/auth"
	"lapangin/internal/booking"
	"lapangin/internal/config"
	"lapangin/internal/database"
	"lapangin/internal/store"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "seed":
		err = withStore(seed)
	case "create-venue":
		err = createVenue(os.Args[2:])
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, `Pemakaian:
  go run ./cmd/manage seed
  go run ./cmd/manage create-venue -slug gor-maju -name "GOR Maju" -owner "Budi" -email budi@contoh.com`)
}

func withStore(fn func(context.Context, *store.Store) error) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx := context.Background()
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	if err := database.Migrate(ctx, pool); err != nil {
		return err
	}
	return fn(ctx, store.New(pool))
}

func everyDay(open, close time.Duration) map[time.Weekday]booking.Hours {
	hours := make(map[time.Weekday]booking.Hours, 7)
	for wd := time.Sunday; wd <= time.Saturday; wd++ {
		hours[wd] = booking.Hours{Open: open, Close: close}
	}
	return hours
}

// Akun demo ini hanya untuk development lokal. Jangan jalankan seed di server production.
const (
	demoEmail    = "demo@lapangin.test"
	demoPassword = "demo12345"
)

func seed(ctx context.Context, st *store.Store) error {
	hash, err := auth.HashPassword(demoPassword)
	if err != nil {
		return err
	}
	_, err = st.CreateVenue(ctx, store.NewVenue{
		Slug:    "demo",
		Name:    "GOR Demo (data contoh)",
		Phone:   "081200000000",
		Address: "Alamat contoh, bukan tempat sungguhan",
		Hours:   everyDay(7*time.Hour, 23*time.Hour),
		Courts: []store.Court{
			{Name: "Lapangan A", Sport: "Futsal", PricePerHour: 150000},
			{Name: "Lapangan B", Sport: "Futsal", PricePerHour: 120000},
			{Name: "Court 1", Sport: "Badminton", PricePerHour: 50000},
		},
		OwnerName:  "Admin Demo",
		OwnerEmail: demoEmail,
		OwnerHash:  hash,
	})
	if errors.Is(err, store.ErrSlugTaken) || errors.Is(err, store.ErrEmailTaken) {
		fmt.Println("Data demo sudah ada, tidak ada yang diubah.")
		return nil
	}
	if err != nil {
		return err
	}
	fmt.Println("Data demo dibuat. Halaman publik: /v/demo, login admin: /admin/login (akun ada di README).")
	return nil
}

var slugPattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func createVenue(args []string) error {
	fs := flag.NewFlagSet("create-venue", flag.ExitOnError)
	slug := fs.String("slug", "", "alamat venue di URL, contoh: gor-maju")
	name := fs.String("name", "", "nama venue")
	owner := fs.String("owner", "", "nama pemilik")
	email := fs.String("email", "", "email login pemilik")
	_ = fs.Parse(args)

	*email = strings.ToLower(strings.TrimSpace(*email))
	switch {
	case !slugPattern.MatchString(*slug):
		return errors.New("-slug wajib, huruf kecil/angka/tanda minus, contoh: gor-maju")
	case strings.TrimSpace(*name) == "":
		return errors.New("-name wajib diisi")
	case strings.TrimSpace(*owner) == "":
		return errors.New("-owner wajib diisi")
	case !strings.Contains(*email, "@"):
		return errors.New("-email tidak valid")
	}

	password, err := readPassword()
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return err
	}

	return withStore(func(ctx context.Context, st *store.Store) error {
		_, err := st.CreateVenue(ctx, store.NewVenue{
			Slug:       *slug,
			Name:       *name,
			Hours:      everyDay(8*time.Hour, 22*time.Hour),
			OwnerName:  *owner,
			OwnerEmail: *email,
			OwnerHash:  hash,
		})
		if err != nil {
			return err
		}
		fmt.Printf("Venue %q dibuat di /v/%s. Pemilik bisa login di /admin/login lalu menambah lapangan.\n", *name, *slug)
		return nil
	})
}

// readPassword meminta password tanpa menampilkannya di layar.
// Kalau input bukan terminal (misalnya dari pipe), dibaca satu baris biasa.
func readPassword() (string, error) {
	fmt.Fprint(os.Stderr, "Password pemilik (min. 8 karakter): ")
	fd := int(os.Stdin.Fd())
	if term.IsTerminal(fd) {
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		return string(b), err
	}
	line, err := bufio.NewReader(os.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
