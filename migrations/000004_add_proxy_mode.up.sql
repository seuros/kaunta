-- Add proxy_mode to website table for per-domain proxy configuration
-- Supports: 'none' (default), 'xforwarded', 'cloudflare'

ALTER TABLE website ADD COLUMN proxy_mode VARCHAR(50) DEFAULT 'none';

-- Add check constraint to validate allowed values
ALTER TABLE website ADD CONSTRAINT check_proxy_mode
  CHECK (proxy_mode IN ('none', 'xforwarded', 'cloudflare'));

-- Create index for potential future filtering by proxy mode
CREATE INDEX IF NOT EXISTS idx_website_proxy_mode ON website(proxy_mode);

-- Add comment for documentation
COMMENT ON COLUMN website.proxy_mode IS 'Proxy mode for IP extraction: none (direct IP), xforwarded (X-Forwarded-For), cloudflare (CF-Connecting-IP)';
