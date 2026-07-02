// Shared money helpers — the single place that knows how to (a) split a Binance pair into base/quote,
// (b) format an amount WITH its currency, and (c) combine per-currency totals into one display
// currency. Fiat renders with the locale's real symbol (R$ 1.234,56 · €1,234.56); stablecoins and
// crypto render as number + asset code (1,234.56 USDT · 0.0042 BTC) — a USDT amount is not US
// dollars, so it never gets a $ sign.

export interface SplitSymbolResult {
  base: string
  quote: string
}

// Fallback quote-asset list for symbols the exchangeInfo map doesn't know (delisted pairs, data from
// another environment). Longest-first so e.g. FDUSD wins over USD.
const FALLBACK_QUOTE_ASSETS = ['FDUSD', 'USDT', 'USDC', 'BUSD', 'TUSD', 'EURI', 'DAI', 'BRL', 'EUR', 'GBP', 'AUD', 'TRY', 'USD', 'BTC', 'ETH', 'BNB'].sort(
  (first, second) => second.length - first.length
)

// splitSymbol("BTCBRL") → { base:"BTC", quote:"BRL" }. Pass the exchange-provided quote as quoteHint
// when known — the suffix guess is only the fallback.
export function splitSymbol(symbol: string, quoteHint?: string): SplitSymbolResult {
  const upper = (symbol || '').toUpperCase()
  const hint = (quoteHint || '').toUpperCase()
  if (hint && upper.length > hint.length && upper.endsWith(hint)) {
    return { base: upper.slice(0, -hint.length), quote: hint }
  }
  for (const quote of FALLBACK_QUOTE_ASSETS) {
    if (upper.length > quote.length && upper.endsWith(quote)) return { base: upper.slice(0, -quote.length), quote }
  }
  return { base: upper, quote: '' }
}

// ISO 4217 currencies Intl can render with a proper symbol (R$, €, £, ₺…).
const FIAT_CODES: Record<string, string> = {
  BRL: 'BRL', USD: 'USD', EUR: 'EUR', GBP: 'GBP', TRY: 'TRY', AUD: 'AUD',
  ARS: 'ARS', MXN: 'MXN', COP: 'COP', JPY: 'JPY', PLN: 'PLN', RON: 'RON',
  UAH: 'UAH', ZAR: 'ZAR', CZK: 'CZK'
}

// Dollar/euro-pegged tokens: 2 decimals like fiat, but labeled with their own code.
const STABLECOIN_CODES = new Set(['USDT', 'USDC', 'BUSD', 'FDUSD', 'TUSD', 'DAI', 'USDP', 'EURI'])

// formatMoney(5544.845, 'BRL', 'pt-BR') → "R$ 5.544,85"; (30, 'USDT', 'en') → "30.00 USDT";
// (0.0042, 'BTC', 'en') → "0.0042 BTC". Empty asset falls back to a plain localized number.
export function formatMoney(value: number, asset: string, locale: string): string {
  if (value === null || value === undefined || !isFinite(value)) return '—'
  const code = (asset || '').toUpperCase()
  if (FIAT_CODES[code]) {
    return new Intl.NumberFormat(locale, { style: 'currency', currency: FIAT_CODES[code] }).format(value)
  }
  if (STABLECOIN_CODES.has(code)) {
    return new Intl.NumberFormat(locale, { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(value) + ' ' + code
  }
  const formatted = new Intl.NumberFormat(locale, { maximumFractionDigits: 8 }).format(value)
  return code ? formatted + ' ' + code : formatted
}

// quoteAssetFor resolves a symbol's quote currency from the exchange map (uppercased key), falling
// back to the suffix guess — the one resolution rule every caller must share, so aggregates and
// per-row labels never bucket the same pair under different currencies.
export function quoteAssetFor(symbol: string, quoteBySymbol: Record<string, string>): string {
  const upper = (symbol || '').toUpperCase()
  return quoteBySymbol[upper] || splitSymbol(upper).quote
}

// A money total broken down by the currency it is denominated in, e.g. { BRL: 812.5, USDT: 30 }.
// '' keys hold amounts whose currency could not be identified.
export type AmountsByCurrency = Record<string, number>

// One amount converted into the display currency — or left in its OWN currency when no rate is known
// (converted=false). The shared convert-or-fallback rule: callers must never sum unconverted parts
// with converted ones (that would mix units); use formatConvertedTotal for totals instead.
export interface ConvertedAmount {
  value: number
  currency: string
  converted: boolean
}

export function convertAmount(value: number, currency: string, displayCode: string, rates: Record<string, number>): ConvertedAmount {
  const rate = currency === displayCode ? 1 : rates[currency]
  if (displayCode && rate && rate > 0) return { value: value * rate, currency: displayCode, converted: true }
  return { value, currency, converted: false }
}

// True when some non-zero part of the total has no rate into the display currency — callers use it
// to withhold judgments (like profit/loss coloring) that only make sense over a fully-converted sum.
export function hasUnconvertedParts(parts: AmountsByCurrency, displayCode: string, rates: Record<string, number>): boolean {
  return Object.entries(parts).some(([currency, value]) => value !== 0 && !convertAmount(value, currency, displayCode, rates).converted)
}

export function subtractAmounts(minuend: AmountsByCurrency, subtrahend: AmountsByCurrency): AmountsByCurrency {
  const result: AmountsByCurrency = { ...minuend }
  for (const [currency, value] of Object.entries(subtrahend)) {
    result[currency] = (result[currency] || 0) - value
  }
  return result
}

// convertedTotalValue sums the parts converted into displayCode with the given rates (quote → display,
// missing/zero rate ⇒ the part is skipped). Used for sign/coloring decisions.
export function convertedTotalValue(parts: AmountsByCurrency, displayCode: string, rates: Record<string, number>): number {
  let sum = 0
  for (const [currency, value] of Object.entries(parts)) {
    const rate = currency === displayCode ? 1 : rates[currency]
    if (rate && rate > 0) sum += value * rate
  }
  return sum
}

// formatConvertedTotal renders a per-currency total as ONE amount in the display currency. Parts with
// no known rate are appended unconverted ("R$ 812,50 + 30.00 USDT") so a missing rate degrades to
// per-currency display instead of silently showing a wrong number.
export function formatConvertedTotal(
  parts: AmountsByCurrency,
  displayCode: string,
  rates: Record<string, number>,
  locale: string,
  signed = false
): string {
  let convertedSum = 0
  let hasConverted = false
  const leftovers: string[] = []
  for (const [currency, value] of Object.entries(parts)) {
    const rate = currency === displayCode ? 1 : rates[currency]
    if (rate && rate > 0) {
      convertedSum += value * rate
      hasConverted = true
    } else if (value !== 0) {
      leftovers.push(formatMoney(value, currency, locale))
    }
  }
  const pieces: string[] = []
  if (hasConverted || leftovers.length === 0) {
    const sign = signed && convertedSum > 0 ? '+' : ''
    pieces.push(sign + formatMoney(convertedSum, displayCode, locale))
  }
  return pieces.concat(leftovers).join(' + ')
}
