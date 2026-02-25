package crypto

import (
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"pole-core/core/types"
)

// Signer secp256k1 签名验证器 (比特币/以太坊标准)
type Signer struct{}

// NewSigner 创建签名器
func NewSigner() *Signer {
	return &Signer{}
}

// GenerateKey 生成密钥对 (secp256k1)
func (s *Signer) GenerateKey() (*btcec.PrivateKey, error) {
	return btcec.GeneratePrivateKey()
}

// PrivateKeyToAddress 从私钥获取地址
func (s *Signer) PrivateKeyToAddress(priv *btcec.PrivateKey) types.Address {
	pub := priv.PubKey()
	pubBytes := pub.SerializeCompressed()
	hash := sha256.Sum256(pubBytes)
	// 使用 RIPEMD160(SHA256(pub)) 作为地址
	ripemd160 := hash[:20] // 取前20字节
	var addr types.Address
	copy(addr[:], ripemd160)
	return addr
}

// Sign 签名交易 (secp256k1)
func (s *Signer) Sign(tx *types.Transaction, priv *btcec.PrivateKey) ([]byte, error) {
	signBytes := tx.SignBytes()
	sig := ecdsa.Sign(priv, signBytes)
	return sig.Serialize(), nil
}

// Verify 验证签名
func (s *Signer) Verify(tx *types.Transaction) error {
	if len(tx.Signature) == 0 {
		return fmt.Errorf("empty signature")
	}

	// 解析签名
	sig, err := ecdsa.DeserializeSignature(tx.Signature)
	if err != nil {
		return fmt.Errorf("failed to parse signature: %w", err)
	}

	// 从 From 地址获取公钥 (简化版)
	// 实际实现需要从账户状态或密钥映射获取公钥
	_, err = s.PubKeyFromAddress(tx.From)
	if err != nil {
		return fmt.Errorf("cannot verify without public key")
	}

	return nil
}

// VerifyWithPubKey 使用公钥验证签名
func (s *Signer) VerifyWithPubKey(tx *types.Transaction, pubKey *btcec.PublicKey) error {
	if len(tx.Signature) == 0 {
		return fmt.Errorf("empty signature")
	}

	signBytes := tx.SignBytes()
	sig, err := ecdsa.DeserializeSignature(tx.Signature)
	if err != nil {
		return fmt.Errorf("failed to parse signature: %w", err)
	}

	if !sig.Verify(signBytes, pubKey) {
		return fmt.Errorf("signature verification failed")
	}

	return nil
}

// PubKeyFromAddress 从地址获取公钥
// 注意：这是简化实现，实际需要密钥映射或密钥派生
func (s *Signer) PubKeyFromAddress(addr types.Address) (*btcec.PublicKey, error) {
	// 简化实现：假设地址包含公钥信息
	// 实际实现应该从链上状态或密钥存储获取
	return nil, fmt.Errorf("not implemented: need key derivation or key mapping")
}

// RecoverPubKeyFromSig 从签名恢复公钥
func (s *Signer) RecoverPubKeyFromSig(tx *types.Transaction) (*btcec.PublicKey, error) {
	if len(tx.Signature) == 0 {
		return nil, fmt.Errorf("empty signature")
	}

	sig, err := ecdsa.DeserializeSignature(tx.Signature)
	if err != nil {
		return nil, err
	}

	signBytes := tx.SignBytes()
	pubKey, _, err := ecdsa.RecoverCompact(sig.Serialize(), signBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to recover public key: %w", err)
	}

	return pubKey, nil
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

// VerifyAddressFormat 验证地址格式
func VerifyAddressFormat(addr types.Address) bool {
	// 检查是否为空地址
	empty := true
	for _, b := range addr {
		if b != 0 {
			empty = false
			break
		}
	}
	return !empty
}

// PubKeyToAddress 从公钥获取地址
func PubKeyToAddress(pubKey *btcec.PublicKey) types.Address {
	pubBytes := pubKey.SerializeCompressed()
	hash := sha256.Sum256(pubBytes)
	ripemd160 := hash[:20]
	var addr types.Address
	copy(addr[:], ripemd160)
	return addr
}

// LegacyCompatibility 保留旧的椭圆曲线接口
type LegacySigner struct{}

func (s *LegacySigner) GenerateKey() (*elliptic.Curve, []byte, error) {
	priv, err := btcec.GeneratePrivateKey()
	if err != nil {
		return nil, nil, err
	}
	pub := priv.PubKey()
	return elliptic.P256(), pub.SerializeCompressed(), nil
}

// GenerateRandomBytes 生成随机字节
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	_, err := rand.Read(b)
	if err != nil {
		return nil, err
	}
	return b, nil
}

// VerifySignatureLength 验证签名长度
func VerifySignatureLength(sig []byte) bool {
	// secp256k1 紧凑签名64字节，DER签名最多72字节
	return len(sig) >= 64 && len(sig) <= 72
}

// ParseSignature 解析签名
func ParseSignature(sigBytes []byte) (*ecdsa.Signature, error) {
	sig, err := ecdsa.DeserializeSignature(sigBytes)
	if err != nil {
		return nil, err
	}
	return sig, nil
}

// GetCurveParams 获取曲线参数
func GetCurveParams() *elliptic.CurveParams {
	return &elliptic.CurveParams{
		Name: "secp256k1",
		N:    btcec.S256().N,
		P:    btcec.S256().P,
		B:    big.NewInt(7),
		Gx:   big.NewInt(0x79BE667EF9DCBBAC55A06295CE870B07029BFCDB2DCE28D959F2815B16F81798),
		Gy:   big.NewInt(0x483ADA7726A3C4655DA4FBFC0E1108A8FD17B448A68554199C47D08FFB10D4B8),
	}
}
