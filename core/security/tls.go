package security

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
	"time"
)

// TLSConfig TLS 1.3 配置
type TLSConfig struct {
	Enabled         bool   // 是否启用 TLS
	CertFile        string // 证书文件路径
	KeyFile         string // 私钥文件路径
	MinVersion      uint16 // 最低 TLS 版本
	MaxVersion      uint16 // 最高 TLS 版本
	VerifyClientCert bool  // 是否验证客户端证书
}

// DefaultTLSConfig 默认 TLS 1.3 配置
func DefaultTLSConfig() *TLSConfig {
	return &TLSConfig{
		Enabled:          false, // 默认不启用，生产环境应开启
		CertFile:         "config/node.crt",
		KeyFile:          "config/node.key",
		MinVersion:       tls.VersionTLS12,
		MaxVersion:       tls.VersionTLS13,
		VerifyClientCert: false,
	}
}

// GenerateSelfSignedCert 生成自签名证书（仅用于测试）
func GenerateSelfSignedCert(certFile, keyFile string, host string, days int) error {
	// 生成 RSA 私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("generate private key: %w", err)
	}

	// 创建证书模板
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := x509.Certificate{
		SerialNumber:          serialNumber,
		Subject: pkix.Name{
			Organization: []string{"PoLE Network"},
			CommonName:   host,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().AddDate(0, 0, days),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// 创建证书
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return fmt.Errorf("create certificate: %w", err)
	}

	// 写入证书文件
	certFileH := certFile
	if err := os.MkdirAll(filepath.Dir(certFileH), 0755); err != nil {
		return err
	}
	certOut, err := os.Create(certFileH)
	if err != nil {
		return fmt.Errorf("create cert file: %w", err)
	}
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	certOut.Close()

	// 写入私钥文件
	keyOut, err := os.Create(keyFile)
	if err != nil {
		return fmt.Errorf("create key file: %w", err)
	}
	keyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: keyBytes})
	keyOut.Close()

	// 设置文件权限
	os.Chmod(certFileH, 0644)
	os.Chmod(keyFile, 0600)

	return nil
}

// LoadTLSConfig 加载或生成 TLS 配置
func LoadTLSConfig(cfg *TLSConfig) (*tls.Config, error) {
	if !cfg.Enabled {
		return nil, nil // 返回 nil 表示不启用 TLS
	}

	// 检查证书文件是否存在
	if _, err := os.Stat(cfg.CertFile); os.IsNotExist(err) {
		// 生成自签名证书
		if err := GenerateSelfSignedCert(cfg.CertFile, cfg.KeyFile, "localhost", 365); err != nil {
			return nil, fmt.Errorf("generate cert: %w", err)
		}
	}

	// 加载证书
	cert, err := tls.LoadX509KeyPair(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load cert: %w", err)
	}

	// 构建 TLS 配置
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   cfg.MinVersion,
		MaxVersion:   cfg.MaxVersion,
	}

	// 客户端证书验证（可选）
	if cfg.VerifyClientCert {
		// 需要加载 CA 证书用于验证客户端
		// 这里简化处理，实际应该从配置加载 CA
		tlsCfg.ClientAuth = tls.RequestClientCert
	}

	return tlsCfg, nil
}

// GetTLSVersionString 获取 TLS 版本字符串
func GetTLSVersionString(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "1.0"
	case tls.VersionTLS11:
		return "1.1"
	case tls.VersionTLS12:
		return "1.2"
	case tls.VersionTLS13:
		return "1.3"
	default:
		return "unknown"
	}
}
