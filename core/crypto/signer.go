package crypto

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"pole-core/core/types"
)

// Signer 交易签名验证器
type Signer struct{}

// NewSigner 创建签名器
func NewSigner() *Signer {
	return &Signer{}
}

// GenerateKey 生成密钥对
func (s *Signer) GenerateKey() (*ecdsa.PrivateKey, error) {
	return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
}

// PrivateKeyToAddress 从私钥获取地址
func (s *Signer) PrivateKeyToAddress(priv *ecdsa.PrivateKey) types.Address {
	pub := priv.PublicKey
	pubBytes := elliptic.Marshal(elliptic.P256(), pub.X, pub.Y)
	hash := sha256.Sum256(pubBytes)
	var addr types.Address
	copy(addr[:], hash[:32])
	return addr
}

// Sign 签名交易
func (s *Signer) Sign(tx *types.Transaction, priv *ecdsa.PrivateKey) ([]byte, error) {
	signBytes := tx.SignBytes()
	r, sig, err := ecdsa.Sign(rand.Reader, priv, signBytes)
	if err != nil {
		return nil, fmt.Errorf("sign: %w", err)
	}
	// 编码签名：R || S
	sigBytes := make([]byte, 0, 64)
	sigBytes = append(sigBytes, r.Bytes()...)
	sigBytes = append(sigBytes, sig.Bytes()...)
	return sigBytes, nil
}

// Verify 验证签名
func (s *Signer) Verify(tx *types.Transaction) error {
	if len(tx.Signature) == 0 {
		return fmt.Errorf("empty signature")
	}
	// 从 From 地址恢复公钥（简化版本：需要额外存储公钥或使用密钥派生）
	// 这里假设 From 已经包含正确的公钥哈希
	// 实际实现需要从 From 派生公钥或使用密钥映射

	// 简化验证：验证签名格式正确
	sigLen := len(tx.Signature)
	if sigLen < 64 || sigLen > 72 { // P256 签名约 64 字节 + DER 编码
		return fmt.Errorf("invalid signature length: %d", sigLen)
	}

	// 解析签名 R 和 S
	r := new(big.Int).SetBytes(tx.Signature[:32])
	ss := new(big.Int).SetBytes(tx.Signature[32:64])

	// 验证 R 和 S 在曲线范围内
	if r.Cmp(elliptic.P256().Params().N) >= 0 || ss.Cmp(elliptic.P256().Params().N) >= 0 {
		return fmt.Errorf("signature point out of range")
	}

	// 注意：这里需要公钥来完整验证
	// 实际实现中，需要从账户状态中获取与 From 地址关联的公钥
	// 或者使用链下密钥派生方案（如 HD Wallet）

	return nil
}

// VerifyWithPubKey 使用公钥验证签名
func (s *Signer) VerifyWithPubKey(tx *types.Transaction, pubKey *ecdsa.PublicKey) error {
	if len(tx.Signature) == 0 {
		return fmt.Errorf("empty signature")
	}

	signBytes := tx.SignBytes()

	// 解析签名 R 和 S
	r := new(big.Int).SetBytes(tx.Signature[:32])
	ss := new(big.Int).SetBytes(tx.Signature[32:64])

	// 使用公钥验证
	if !ecdsa.Verify(pubKey, signBytes, r, ss) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

// PubKeyFromAddress 从地址获取公钥（简化版）
// 实际实现需要密钥映射或密钥派生
func (s *Signer) PubKeyFromAddress(addr types.Address) (*ecdsa.PublicKey, error) {
	// 简化：从地址派生（不安全，仅用于演示）
	// 实际实现需要维护地址到公钥的映射
	return nil, fmt.Errorf("not implemented: need key derivation scheme")
}

// ==================== 工具函数 ====================

// BytesToAddress 将字节转换为地址
func BytesToAddress(b []byte) types.Address {
	var addr types.Address
	if len(b) > 32 {
		b = b[:32]
	}
	copy(addr[:], b)
	return addr
}

// AddressToBytes 将地址转换为字节
func AddressToBytes(addr types.Address) []byte {
	return addr[:]
}

// AddressToString 将地址转换为字符串
func AddressToString(addr types.Address) string {
	return hex.EncodeToString(addr[:])
}

// StringToAddress 将字符串转换为地址
func StringToAddress(s string) (types.Address, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return types.Address{}, err
	}
	return BytesToAddress(b), nil
}
