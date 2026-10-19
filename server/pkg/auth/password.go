package auth

import "golang.org/x/crypto/bcrypt"

// HashPassword 用 bcrypt 生成密码哈希。
func (a *Auth) HashPassword(pwd string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pwd), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword 校验明文密码与 bcrypt 哈希是否匹配。
func (a *Auth) CheckPassword(hash, pwd string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pwd)) == nil
}
