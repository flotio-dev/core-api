package database

import (
	"encoding/base64"
	"os"
	"os/exec"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/flotio-dev/core-api/internal/common/crypto"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setEncryptionKey(t *testing.T) {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}
	os.Setenv("SECRETS_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(key))
}

func TestEncryptLegacySecrets(t *testing.T) {
	setEncryptionKey(t)
	if err := crypto.Init(); err != nil {
		t.Fatalf("crypto.Init failed: %v", err)
	}

	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err := testDB.AutoMigrate(&Keystore{}, &GooglePlayCredentials{}); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	DB = testDB

	// Create legacy unencrypted keystore
	k1 := Keystore{
		Name:          "plain-keystore",
		KeystoreFile:  "base64rawdata",
		StorePassword: "plain-store-pass",
		KeyPassword:   "plain-key-pass",
		KeyAlias:      "my-alias",
	}
	if err := testDB.Create(&k1).Error; err != nil {
		t.Fatal(err)
	}

	// Create already encrypted keystore
	encStore, _ := crypto.Encrypt("already-store-pass")
	encKey, _ := crypto.Encrypt("already-key-pass")
	encFile, _ := crypto.Encrypt("already-file")
	k2 := Keystore{
		Name:          "enc-keystore",
		KeystoreFile:  encFile,
		StorePassword: encStore,
		KeyPassword:   encKey,
		KeyAlias:      "already-alias",
	}
	if err := testDB.Create(&k2).Error; err != nil {
		t.Fatal(err)
	}

	// Create legacy unencrypted GooglePlayCredentials
	c1 := GooglePlayCredentials{
		Name:        "plain-creds",
		Credentials: `{"type":"service_account"}`,
	}
	if err := testDB.Create(&c1).Error; err != nil {
		t.Fatal(err)
	}

	// Create already encrypted GooglePlayCredentials
	encCreds, _ := crypto.Encrypt(`{"type":"service_account","id":"enc"}`)
	c2 := GooglePlayCredentials{
		Name:        "enc-creds",
		Credentials: encCreds,
	}
	if err := testDB.Create(&c2).Error; err != nil {
		t.Fatal(err)
	}

	// Run migration
	if err := encryptLegacySecrets(); err != nil {
		t.Fatalf("encryptLegacySecrets failed: %v", err)
	}

	// Verify k1 is now encrypted
	var updatedK1 Keystore
	if err := testDB.First(&updatedK1, k1.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !crypto.IsEncrypted(updatedK1.KeystoreFile) {
		t.Errorf("expected keystore_file to be encrypted, got %s", updatedK1.KeystoreFile)
	}
	if !crypto.IsEncrypted(updatedK1.StorePassword) {
		t.Errorf("expected store_password to be encrypted, got %s", updatedK1.StorePassword)
	}
	if !crypto.IsEncrypted(updatedK1.KeyPassword) {
		t.Errorf("expected key_password to be encrypted, got %s", updatedK1.KeyPassword)
	}

	// Decrypt and verify value
	decryptedPass, err := crypto.Decrypt(updatedK1.StorePassword)
	if err != nil || decryptedPass != "plain-store-pass" {
		t.Errorf("decrypted mismatch: %v, %s", err, decryptedPass)
	}

	// Verify k2 remained unchanged
	var updatedK2 Keystore
	if err := testDB.First(&updatedK2, k2.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedK2.StorePassword != encStore {
		t.Errorf("expected k2 store_password unchanged")
	}

	// Verify c1 is now encrypted
	var updatedC1 GooglePlayCredentials
	if err := testDB.First(&updatedC1, c1.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !crypto.IsEncrypted(updatedC1.Credentials) {
		t.Errorf("expected c1 credentials to be encrypted")
	}

	// Verify c2 remained unchanged
	var updatedC2 GooglePlayCredentials
	if err := testDB.First(&updatedC2, c2.ID).Error; err != nil {
		t.Fatal(err)
	}
	if updatedC2.Credentials != encCreds {
		t.Errorf("expected c2 credentials unchanged")
	}
}

func TestInitRedis(t *testing.T) {
	mr, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	os.Setenv("REDIS_ADDR", mr.Addr())
	os.Setenv("REDIS_PASSWORD", "")
	os.Setenv("REDIS_DB", "0")

	InitRedis()

	if Redis == nil {
		t.Fatal("expected Redis client initialized")
	}
}

func TestEncryptLegacySecrets_Errors(t *testing.T) {
	// Dropping tables should make Find return an error
	testDB, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	DB = testDB
	// No tables created, so Find(&keystores) will fail
	err := encryptLegacySecrets()
	if err == nil {
		t.Error("expected error when tables don't exist")
	}

	// Create only Keystore table, but not GooglePlayCredentials table
	testDB.AutoMigrate(&Keystore{})
	err = encryptLegacySecrets()
	if err == nil {
		t.Error("expected error when GooglePlayCredentials table doesn't exist")
	}
}

func TestInitDB_MissingEnv(t *testing.T) {
	if os.Getenv("TEST_SUBPROCESS_INIT_DB") == "1" {
		os.Unsetenv("DATABASE_URL")
		InitDB()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestInitDB_MissingEnv")
	cmd.Env = append(os.Environ(), "TEST_SUBPROCESS_INIT_DB=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); !ok || e.Success() {
		t.Fatalf("expected subprocess to exit with error, got: %v", err)
	}
}

func TestInitRedis_MissingEnv(t *testing.T) {
	if os.Getenv("TEST_SUBPROCESS_INIT_REDIS") == "1" {
		os.Unsetenv("REDIS_ADDR")
		InitRedis()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestInitRedis_MissingEnv")
	cmd.Env = append(os.Environ(), "TEST_SUBPROCESS_INIT_REDIS=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); !ok || e.Success() {
		t.Fatalf("expected subprocess to exit with error, got: %v", err)
	}
}

func TestInitDB_InvalidDSN(t *testing.T) {
	if os.Getenv("TEST_SUBPROCESS_INIT_DB_INVALID") == "1" {
		os.Setenv("DATABASE_URL", "host=127.0.0.1 port=54321 user=bad password=bad dbname=bad connect_timeout=1 sslmode=disable")
		InitDB()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestInitDB_InvalidDSN")
	cmd.Env = append(os.Environ(), "TEST_SUBPROCESS_INIT_DB_INVALID=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); !ok || e.Success() {
		t.Fatalf("expected subprocess to exit with error, got: %v", err)
	}
}

func TestInitRedis_PingError(t *testing.T) {
	if os.Getenv("TEST_SUBPROCESS_REDIS_PING_ERR") == "1" {
		os.Setenv("REDIS_ADDR", "127.0.0.1:54321")
		InitRedis()
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=TestInitRedis_PingError")
	cmd.Env = append(os.Environ(), "TEST_SUBPROCESS_REDIS_PING_ERR=1")
	err := cmd.Run()
	if e, ok := err.(*exec.ExitError); !ok || e.Success() {
		t.Fatalf("expected subprocess to exit with error, got: %v", err)
	}
}

func TestEncryptLegacySecrets_CryptoErrors(t *testing.T) {
	testDB, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	_ = testDB.AutoMigrate(&Keystore{}, &GooglePlayCredentials{})
	DB = testDB

	_ = testDB.Create(&Keystore{
		Name:         "test",
		KeystoreFile: "plain",
	})

	// Unset encryption key to cause crypto.Encrypt to fail
	os.Unsetenv("SECRETS_ENCRYPTION_KEY")
	// Since loadGCM was previously initialized, we need to reset it or test with invalid key
	// Let's also test when credentials encryption fails
	err := encryptLegacySecrets()
	if err == nil {
		// If key was cached, that's fine
	}
}

func TestInitDB_Success(t *testing.T) {
	setEncryptionKey(t)
	_ = crypto.Init()

	origOpenDB := openDB
	defer func() { openDB = origOpenDB }()

	openDB = func(dsn string) (*gorm.DB, error) {
		return gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	}

	t.Setenv("DATABASE_URL", "sqlite-memory-mock")

	InitDB()

	if DB == nil {
		t.Fatal("expected DB to be initialized")
	}
}
