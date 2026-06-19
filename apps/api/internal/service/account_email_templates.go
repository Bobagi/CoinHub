package service

import (
	"fmt"
	"html"
	"strings"

	"coin-hub/internal/email"
)

// normalizeEmailLocale keeps the email language to one we have copy for, defaulting to pt-BR.
func normalizeEmailLocale(locale string) string {
	switch locale {
	case "en", "es", "pt":
		return locale
	default:
		return "pt"
	}
}

func passwordResetEmail(locale string, link string) email.Message {
	switch normalizeEmailLocale(locale) {
	case "en":
		return email.Message{
			Subject:  "Coin Hub — reset your password",
			TextBody: "You asked to reset your Coin Hub password. Open the link below (valid for 1 hour):\n\n" + link + "\n\nIf this wasn't you, just ignore this email — your password stays the same.",
			HTMLBody: brandedEmailHTML("Reset your password", "You asked to reset your Coin Hub password. Click the button below to choose a new one. The link is valid for 1 hour.", "Reset password", link, "If this wasn't you, just ignore this email — your password stays the same."),
		}
	case "es":
		return email.Message{
			Subject:  "Coin Hub — restablecer tu contraseña",
			TextBody: "Solicitaste restablecer tu contraseña de Coin Hub. Abre el enlace de abajo (válido por 1 hora):\n\n" + link + "\n\nSi no fuiste tú, ignora este correo — tu contraseña no cambia.",
			HTMLBody: brandedEmailHTML("Restablecer tu contraseña", "Solicitaste restablecer tu contraseña de Coin Hub. Haz clic en el botón para elegir una nueva. El enlace es válido por 1 hora.", "Restablecer contraseña", link, "Si no fuiste tú, ignora este correo — tu contraseña no cambia."),
		}
	default:
		return email.Message{
			Subject:  "Coin Hub — redefinição de senha",
			TextBody: "Você pediu para redefinir sua senha no Coin Hub. Abra o link abaixo (válido por 1 hora):\n\n" + link + "\n\nSe não foi você, ignore este e-mail — sua senha continua a mesma.",
			HTMLBody: brandedEmailHTML("Redefinir sua senha", "Você pediu para redefinir sua senha no Coin Hub. Clique no botão para escolher uma nova. O link é válido por 1 hora.", "Redefinir senha", link, "Se não foi você, ignore este e-mail — sua senha continua a mesma."),
		}
	}
}

func emailVerificationEmail(locale string, link string) email.Message {
	switch normalizeEmailLocale(locale) {
	case "en":
		return email.Message{
			Subject:  "Coin Hub — confirm your email",
			TextBody: "Welcome to Coin Hub! Confirm your email by opening the link below (valid for 24 hours):\n\n" + link + "\n\nIf you didn't create an account, just ignore this email.",
			HTMLBody: brandedEmailHTML("Confirm your email", "Welcome to Coin Hub! Confirm your email address so we know it's really you. The link is valid for 24 hours.", "Confirm email", link, "If you didn't create an account, just ignore this email."),
		}
	case "es":
		return email.Message{
			Subject:  "Coin Hub — confirma tu correo",
			TextBody: "¡Bienvenido a Coin Hub! Confirma tu correo abriendo el enlace de abajo (válido por 24 horas):\n\n" + link + "\n\nSi no creaste una cuenta, ignora este correo.",
			HTMLBody: brandedEmailHTML("Confirma tu correo", "¡Bienvenido a Coin Hub! Confirma tu dirección de correo para que sepamos que eres tú. El enlace es válido por 24 horas.", "Confirmar correo", link, "Si no creaste una cuenta, ignora este correo."),
		}
	default:
		return email.Message{
			Subject:  "Coin Hub — confirme seu e-mail",
			TextBody: "Bem-vindo ao Coin Hub! Confirme seu e-mail abrindo o link abaixo (válido por 24 horas):\n\n" + link + "\n\nSe você não criou uma conta, ignore este e-mail.",
			HTMLBody: brandedEmailHTML("Confirme seu e-mail", "Bem-vindo ao Coin Hub! Confirme seu endereço de e-mail para sabermos que é você mesmo. O link é válido por 24 horas.", "Confirmar e-mail", link, "Se você não criou uma conta, ignore este e-mail."),
		}
	}
}

// newAccessAlertEmail is the security notice sent when a new (unrecognized) device or network signs
// in. It lists the device, IP and time so the user can judge whether the access was theirs.
func newAccessAlertEmail(locale string, device string, ipAddress string, location string, whenText string, accountLink string) email.Message {
	deviceValue := valueOrDash(device)
	ipValue := valueOrDash(ipAddress)
	locationValue := valueOrDash(location)
	whenValue := valueOrDash(whenText)
	switch normalizeEmailLocale(locale) {
	case "en":
		return email.Message{
			Subject:  "Coin Hub — new sign-in to your account",
			TextBody: "We noticed a sign-in to your Coin Hub account from a device or network we hadn't seen before.\n\nDevice: " + deviceValue + "\nLocation: " + locationValue + "\nIP address: " + ipValue + "\nWhen: " + whenValue + "\n\nReview your account: " + accountLink + "\n\nIf this was you, you can ignore this email. If you don't recognize it, change your password right away.",
			HTMLBody: brandedAlertEmailHTML("New sign-in detected", "We noticed a sign-in to your Coin Hub account from a device or network we hadn't seen before.", [][2]string{{"Device", deviceValue}, {"Location", locationValue}, {"IP address", ipValue}, {"When", whenValue}}, "Review your account", accountLink, "If this was you, you can ignore this email. If you don't recognize it, change your password right away."),
		}
	case "es":
		return email.Message{
			Subject:  "Coin Hub — nuevo acceso a tu cuenta",
			TextBody: "Detectamos un acceso a tu cuenta de Coin Hub desde un dispositivo o red que no habíamos visto antes.\n\nDispositivo: " + deviceValue + "\nUbicación: " + locationValue + "\nDirección IP: " + ipValue + "\nCuándo: " + whenValue + "\n\nRevisar tu cuenta: " + accountLink + "\n\nSi fuiste tú, puedes ignorar este correo. Si no reconoces este acceso, cambia tu contraseña de inmediato.",
			HTMLBody: brandedAlertEmailHTML("Nuevo acceso detectado", "Detectamos un acceso a tu cuenta de Coin Hub desde un dispositivo o red que no habíamos visto antes.", [][2]string{{"Dispositivo", deviceValue}, {"Ubicación", locationValue}, {"Dirección IP", ipValue}, {"Cuándo", whenValue}}, "Revisar tu cuenta", accountLink, "Si fuiste tú, puedes ignorar este correo. Si no reconoces este acceso, cambia tu contraseña de inmediato."),
		}
	default:
		return email.Message{
			Subject:  "Coin Hub — novo acesso à sua conta",
			TextBody: "Detectamos um acesso à sua conta Coin Hub a partir de um dispositivo ou rede que ainda não conhecíamos.\n\nDispositivo: " + deviceValue + "\nLocal: " + locationValue + "\nEndereço IP: " + ipValue + "\nQuando: " + whenValue + "\n\nRevisar sua conta: " + accountLink + "\n\nSe foi você, pode ignorar este e-mail. Se não reconhece este acesso, troque sua senha imediatamente.",
			HTMLBody: brandedAlertEmailHTML("Novo acesso detectado", "Detectamos um acesso à sua conta Coin Hub a partir de um dispositivo ou rede que ainda não conhecíamos.", [][2]string{{"Dispositivo", deviceValue}, {"Local", locationValue}, {"Endereço IP", ipValue}, {"Quando", whenValue}}, "Revisar sua conta", accountLink, "Se foi você, pode ignorar este e-mail. Se não reconhece este acesso, troque sua senha imediatamente."),
		}
	}
}

// valueOrDash returns an em dash for empty/blank values so the email never shows a blank field.
func valueOrDash(value string) string {
	if strings.TrimSpace(value) == "" {
		return "—"
	}
	return value
}

// brandedAlertEmailHTML renders the security-alert email: the brand shell plus a labelled details
// table (device / IP / when). Labels and values are HTML-escaped.
func brandedAlertEmailHTML(heading string, intro string, details [][2]string, buttonLabel string, link string, footer string) string {
	var detailRows strings.Builder
	for _, detail := range details {
		detailRows.WriteString(fmt.Sprintf(
			`<tr><td style="font-size:13px;color:#a89f8c;padding:4px 12px 4px 0;white-space:nowrap;vertical-align:top;">%s</td><td style="font-size:13px;color:#e9e2cf;padding:4px 0;word-break:break-all;">%s</td></tr>`,
			html.EscapeString(detail[0]), html.EscapeString(detail[1]),
		))
	}
	return fmt.Sprintf(`<!doctype html>
<html><body style="margin:0;background:#1a1714;font-family:Segoe UI,Arial,sans-serif;color:#fff9db;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:#1a1714;padding:32px 0;">
    <tr><td align="center">
      <table role="presentation" width="480" cellpadding="0" cellspacing="0" style="max-width:480px;background:#231f1b;border:1px solid #3a332b;border-radius:14px;padding:32px;">
        <tr><td style="font-size:22px;font-weight:800;color:#ffd43b;padding-bottom:16px;">Coin&nbsp;Hub</td></tr>
        <tr><td style="font-size:18px;font-weight:700;padding-bottom:12px;">%s</td></tr>
        <tr><td style="font-size:15px;line-height:1.6;color:#e9e2cf;padding-bottom:20px;">%s</td></tr>
        <tr><td style="padding-bottom:24px;">
          <table role="presentation" cellpadding="0" cellspacing="0" style="background:#1a1714;border:1px solid #3a332b;border-radius:10px;padding:12px 16px;">%s</table>
        </td></tr>
        <tr><td style="padding-bottom:24px;"><a href="%s" style="display:inline-block;background:#ffd43b;color:#1a1714;font-weight:800;text-decoration:none;padding:12px 22px;border-radius:10px;">%s</a></td></tr>
        <tr><td style="font-size:13px;line-height:1.6;color:#a89f8c;">%s</td></tr>
      </table>
    </td></tr>
  </table>
</body></html>`, html.EscapeString(heading), html.EscapeString(intro), detailRows.String(), html.EscapeString(link), html.EscapeString(buttonLabel), html.EscapeString(footer))
}

// brandedEmailHTML renders a simple, inline-styled email matching the warm-dark + gold brand. Links
// and text are HTML-escaped.
func brandedEmailHTML(heading string, paragraph string, buttonLabel string, link string, footer string) string {
	safeHeading := html.EscapeString(heading)
	safeParagraph := html.EscapeString(paragraph)
	safeButton := html.EscapeString(buttonLabel)
	safeLink := html.EscapeString(link)
	safeFooter := html.EscapeString(footer)
	return fmt.Sprintf(`<!doctype html>
<html><body style="margin:0;background:#1a1714;font-family:Segoe UI,Arial,sans-serif;color:#fff9db;">
  <table role="presentation" width="100%%" cellpadding="0" cellspacing="0" style="background:#1a1714;padding:32px 0;">
    <tr><td align="center">
      <table role="presentation" width="480" cellpadding="0" cellspacing="0" style="max-width:480px;background:#231f1b;border:1px solid #3a332b;border-radius:14px;padding:32px;">
        <tr><td style="font-size:22px;font-weight:800;color:#ffd43b;padding-bottom:16px;">Coin&nbsp;Hub</td></tr>
        <tr><td style="font-size:18px;font-weight:700;padding-bottom:12px;">%s</td></tr>
        <tr><td style="font-size:15px;line-height:1.6;color:#e9e2cf;padding-bottom:24px;">%s</td></tr>
        <tr><td style="padding-bottom:24px;"><a href="%s" style="display:inline-block;background:#ffd43b;color:#1a1714;font-weight:800;text-decoration:none;padding:12px 22px;border-radius:10px;">%s</a></td></tr>
        <tr><td style="font-size:13px;line-height:1.6;color:#a89f8c;word-break:break-all;">%s<br><br><a href="%s" style="color:#ffd43b;">%s</a></td></tr>
      </table>
    </td></tr>
  </table>
</body></html>`, safeHeading, safeParagraph, safeLink, safeButton, safeFooter, safeLink, safeLink)
}
