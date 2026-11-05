-- 002_weather_alert_history.sql
-- Stores history of weather alerts triggered across Chonburi subdistricts
-- Used by Mobile App to render the in-app Alert Feed

CREATE TABLE IF NOT EXISTS weather_alert_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subdistrict VARCHAR(100) NOT NULL,
    district VARCHAR(100) NOT NULL,
    title VARCHAR(255) NOT NULL,
    message TEXT NOT NULL,
    severity VARCHAR(20) NOT NULL DEFAULT 'info',
    alert_type VARCHAR(50) NOT NULL DEFAULT 'general',
    image_url TEXT,
    sent_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wah_subdistrict ON weather_alert_history(subdistrict);
CREATE INDEX IF NOT EXISTS idx_wah_created_at_desc ON weather_alert_history(created_at DESC);

