-- ============================================================
-- Migration: user_weather_subscriptions
-- Target DB: Chonburi Shared PostgreSQL
-- Used by:
--   WRITE -> super-app-chonburi-go-mobile (Mobile API)
--   READ  -> super-app-chonburi-go (Alert Evaluation Cron)
-- ============================================================

CREATE TABLE IF NOT EXISTS user_weather_subscriptions (
    id          UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID        NOT NULL,
    district    TEXT        NOT NULL,
    subdistrict TEXT        NOT NULL,
    is_primary  BOOLEAN     NOT NULL DEFAULT FALSE,
    is_active   BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    -- Prevent duplicate subscription per user+subdistrict
    CONSTRAINT uq_uws_user_subdistrict UNIQUE (user_id, subdistrict)
);

-- Index A: Alert Engine — query users by subdistrict (Critical Hot Path)
-- Partial index on is_active=TRUE reduces index size significantly
CREATE INDEX IF NOT EXISTS idx_uws_subdistrict_active
    ON user_weather_subscriptions (subdistrict, is_active)
    WHERE is_active = TRUE;

-- Index B: Mobile API — fetch subscriptions by user_id
CREATE INDEX IF NOT EXISTS idx_uws_user_id
    ON user_weather_subscriptions (user_id);

-- Auto-update updated_at on row change
CREATE OR REPLACE FUNCTION fn_set_updated_at()
    RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_uws_updated_at ON user_weather_subscriptions;
CREATE TRIGGER trg_uws_updated_at
    BEFORE UPDATE ON user_weather_subscriptions
    FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();

