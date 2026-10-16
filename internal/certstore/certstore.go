// Package certstore 负责本地根 CA 的生成/加载、按 SNI 动态签发叶子证书，
// 以及安装/卸载到系统根证书存储（Windows: certutil；macOS: security 钥匙串）。
// 取代旧版对 mitmproxy CA 的依赖。
package certstore

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"bailan-relay/internal/config"
)

// CA 持有根证书与私钥，并缓存已签发的叶子证书。
type CA struct {
	cert    *x509.Certificate
	certDER []byte
	key     *rsa.PrivateKey

	leafMu  sync.Mutex
	leafKey *rsa.PrivateKey
	leaves  map[string]*tls.Certificate
}

// LoadOrCreate 从磁盘加载 CA，不存在则生成并落盘。
func LoadOrCreate() (*CA, error) {
	if ca, err := load(); err == nil {
		ca.leaves = map[string]*tls.Certificate{}
		return ca, nil
	}
	return generate()
}

func load() (*CA, error) {
	der, err := os.ReadFile(config.CertFile())
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	keyPEM, err := os.ReadFile(config.CertKeyFile())
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(keyPEM)
	if block == nil {
		return nil, fmt.Errorf("私钥 PEM 解析失败")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	return &CA{cert: cert, certDER: der, key: key}, nil
}

func generate() (*CA, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   config.CertCN(),
			Organization: []string{"BaiLan"},
		},
		NotBefore:             time.Now().Add(-24 * time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, err
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(config.CertDir(), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(config.CertFile(), der, 0o644); err != nil {
		return nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(config.CertKeyFile(), keyPEM, 0o600); err != nil {
		return nil, err
	}
	return &CA{cert: cert, certDER: der, key: key, leaves: map[string]*tls.Certificate{}}, nil
}

// GetCertificate 供 tls.Config 使用：按 SNI 动态签发/缓存叶子证书。
func (c *CA) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	host := hello.ServerName
	if host == "" {
		host = "localhost"
	}
	c.leafMu.Lock()
	defer c.leafMu.Unlock()
	if cert, ok := c.leaves[host]; ok {
		return cert, nil
	}
	if c.leafKey == nil {
		key, err := rsa.GenerateKey(rand.Reader, 2048)
		if err != nil {
			return nil, err
		}
		c.leafKey = key
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host},
		DNSNames:     []string{host},
		NotBefore:    time.Now().Add(-24 * time.Hour),
		NotAfter:     time.Now().AddDate(0, 0, 397),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, c.cert, &c.leafKey.PublicKey, c.key)
	if err != nil {
		return nil, err
	}
	cert := &tls.Certificate{
		Certificate: [][]byte{der, c.certDER},
		PrivateKey:  c.leafKey,
	}
	c.leaves[host] = cert
	return cert, nil
}

// FileExists 返回 CA 证书文件是否存在。
func FileExists() bool {
	_, err := os.Stat(config.CertFile())
	return err == nil
}

// CertPath 返回 CA 证书文件路径（供 UI 显示）。
func CertPath() string { return filepath.Clean(config.CertFile()) }

// EnsureInstalled 已信任则跳过，否则安装。输出：(是否已就绪, 日志文案)。
func EnsureInstalled() (bool, string) {
	if ok, _ := IsInstalled(); ok {
		return true, "根证书已在系统信任库中"
	}
	if ok, msg := Install(); !ok {
		return false, "根证书自动安装失败：" + strings.TrimSpace(msg)
	}
	return true, "根证书已自动安装到系统信任库"
}

// AutoUninstall 卸载根证书并返回日志文案（未安装视为成功）。
func AutoUninstall() string {
	if ok, _ := IsInstalled(); !ok {
		return "根证书未安装，无需卸载"
	}
	if ok, msg := Uninstall(); !ok {
		return "根证书自动卸载失败：" + strings.TrimSpace(msg)
	}
	return "根证书已从系统信任库卸载"
}
