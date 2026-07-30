package util

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"time"

	"gopkg.in/gomail.v2"
)

// EmailConfig 邮件配置
type EmailConfig struct {
	SMTPHost     string // SMTP 服务器地址，如 smtp.qq.com
	SMTPPort     int    // SMTP 端口，一般是 465 或 587
	SenderEmail  string // 发件人邮箱
	SenderName   string // 发件人名称
	AuthPassword string // 授权码（不是邮箱登录密码）
}

// 默认邮件配置（建议从环境变量或配置文件读取）
var defaultEmailConfig = EmailConfig{
	SMTPHost:     "smtp.qq.com", // QQ 邮箱 SMTP 服务器
	SMTPPort:     465,           // 使用 SSL
	SenderEmail:  "",            // 需要配置
	SenderName:   "聊天服务器",
	AuthPassword: "", // 需要配置授权码
}

// SetEmailConfig 设置邮件配置
func SetEmailConfig(config EmailConfig) {
	defaultEmailConfig = config
}

// GetEmailConfig 获取当前邮件配置
func GetEmailConfig() EmailConfig {
	return defaultEmailConfig
}

// GenerateVerifyCode 生成指定位数的数字验证码
// length: 验证码长度，推荐 6 位
func GenerateVerifyCode(length int) (string, error) {
	if length <= 0 {
		length = 6 // 默认 6 位
	}

	const digits = "0123456789"
	code := make([]byte, length)

	for i := 0; i < length; i++ {
		// 使用加密安全的随机数生成器
		num, err := rand.Int(rand.Reader, big.NewInt(int64(len(digits))))
		if err != nil {
			return "", fmt.Errorf("生成验证码失败: %w", err)
		}
		code[i] = digits[num.Int64()]
	}

	return string(code), nil
}

// SendVerifyCodeEmail 发送验证码邮件
// toEmail: 收件人邮箱
// code: 验证码
// expireMinutes: 验证码有效期（分钟）
func SendVerifyCodeEmail(toEmail, code string, expireMinutes int) error {
	config := defaultEmailConfig

	// 检查配置
	if config.SenderEmail == "" || config.AuthPassword == "" {
		return fmt.Errorf("邮件配置不完整，请先调用 SetEmailConfig 设置发件人邮箱和授权码")
	}

	// 构建邮件内容
	subject := "【聊天服务器】验证码"
	body := buildVerifyCodeEmailBody(code, expireMinutes)

	// 发送邮件
	return sendEmail(config, toEmail, subject, body)
}

// buildVerifyCodeEmailBody 构建验证码邮件内容（HTML 格式）
func buildVerifyCodeEmailBody(code string, expireMinutes int) string {
	return fmt.Sprintf(`
<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <style>
        body { font-family: Arial, sans-serif; line-height: 1.6; color: #333; }
        .container { max-width: 600px; margin: 0 auto; padding: 20px; }
        .header { background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); color: white; padding: 30px; text-align: center; border-radius: 10px 10px 0 0; }
        .content { background: #f9f9f9; padding: 30px; border-radius: 0 0 10px 10px; }
        .code-box { background: white; border: 2px dashed #667eea; padding: 20px; text-align: center; margin: 20px 0; border-radius: 8px; }
        .code { font-size: 32px; font-weight: bold; color: #667eea; letter-spacing: 5px; }
        .tips { color: #666; font-size: 14px; margin-top: 20px; }
        .warning { color: #e74c3c; font-weight: bold; }
        .footer { text-align: center; color: #999; font-size: 12px; margin-top: 20px; }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🔐 验证码</h1>
        </div>
        <div class="content">
            <p>您好，</p>
            <p>您正在进行身份验证，您的验证码是：</p>
            <div class="code-box">
                <div class="code">%s</div>
            </div>
            <div class="tips">
                <p>• 验证码有效期为 <strong>%d 分钟</strong></p>
                <p>• 请勿将验证码告知他人</p>
                <p class="warning">• 如非本人操作，请忽略此邮件</p>
            </div>
        </div>
        <div class="footer">
            <p>此邮件由系统自动发送，请勿回复</p>
            <p>&copy; %d 聊天服务器 版权所有</p>
        </div>
    </div>
</body>
</html>
`, code, expireMinutes, time.Now().Year())
}

// sendEmail 发送邮件的底层函数
func sendEmail(config EmailConfig, toEmail, subject, body string) error {
	// 创建邮件消息
	m := gomail.NewMessage()

	// 设置发件人
	m.SetHeader("From", m.FormatAddress(config.SenderEmail, config.SenderName))

	// 设置收件人
	m.SetHeader("To", toEmail)

	// 设置主题
	m.SetHeader("Subject", subject)

	// 设置邮件正文（HTML 格式）
	m.SetBody("text/html", body)

	// 创建 SMTP 拨号器
	d := gomail.NewDialer(config.SMTPHost, config.SMTPPort, config.SenderEmail, config.AuthPassword)

	// 发送邮件
	if err := d.DialAndSend(m); err != nil {
		return fmt.Errorf("发送邮件失败: %w", err)
	}

	return nil
}

// SendCustomEmail 发送自定义内容的邮件
// toEmail: 收件人邮箱
// subject: 邮件主题
// body: 邮件内容（支持 HTML）
func SendCustomEmail(toEmail, subject, body string) error {
	config := defaultEmailConfig

	if config.SenderEmail == "" || config.AuthPassword == "" {
		return fmt.Errorf("邮件配置不完整，请先调用 SetEmailConfig 设置发件人邮箱和授权码")
	}

	return sendEmail(config, toEmail, subject, body)
}

// ValidateEmail 简单的邮箱格式验证
func ValidateEmail(email string) bool {
	// 简单验证：包含 @ 且 @ 后有 .
	if len(email) < 5 {
		return false
	}

	atIndex := -1
	dotIndex := -1

	for i, ch := range email {
		if ch == '@' {
			if atIndex != -1 {
				return false // 多个 @
			}
			atIndex = i
		}

		if ch == '.' && atIndex != -1 {
			dotIndex = i
		}
	}

	// @ 必须存在，且 . 必须在 @ 之后
	return atIndex > 0 && dotIndex > atIndex+1 && dotIndex < len(email)-1
}

// GetCommonSMTPConfig 获取常见邮箱的 SMTP 配置
func GetCommonSMTPConfig(provider string) (host string, port int) {
	configs := map[string]struct {
		host string
		port int
	}{
		"qq":      {"smtp.qq.com", 465},
		"163":     {"smtp.163.com", 465},
		"126":     {"smtp.126.com", 465},
		"gmail":   {"smtp.gmail.com", 587},
		"outlook": {"smtp.office365.com", 587},
	}

	if config, ok := configs[provider]; ok {
		return config.host, config.port
	}

	// 默认返回 QQ 邮箱配置
	return "smtp.qq.com", 465
}
