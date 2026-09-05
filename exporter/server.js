import express from 'express'
import { chromium } from 'playwright'

const app = express()
const port = Number(process.env.PORT || 3000)
app.use(express.json({ limit: '32kb' }))

app.get('/healthz', (_request, response) => response.json({ status: 'ok' }))

app.post('/render', async (request, response) => {
  const { url } = request.body ?? {}
  if (typeof url !== 'string' || !/^https?:\/\//.test(url)) {
    return response.status(400).json({ error: 'url is required' })
  }

  let browser
  try {
    browser = await chromium.launch({ headless: true })
    const page = await browser.newPage({ viewport: { width: 1440, height: 900 }, deviceScaleFactor: 1 })
    await page.goto(url, { waitUntil: 'networkidle', timeout: 60000 })
    await page.emulateMedia({ media: 'print' })
    await page.evaluate(async () => {
      if (document.fonts?.ready) await document.fonts.ready
      await new Promise((resolve) => setTimeout(resolve, 300))
    })
    const pdf = await page.pdf({
      format: 'A4',
      printBackground: true,
      preferCSSPageSize: true,
      margin: { top: '12mm', right: '12mm', bottom: '12mm', left: '12mm' },
    })
    response.type('application/pdf').send(pdf)
  } catch (error) {
    console.error(error)
    response.status(502).json({ error: 'failed to render pdf' })
  } finally {
    await browser?.close()
  }
})

app.listen(port, () => console.log(`exporter listening on ${port}`))

