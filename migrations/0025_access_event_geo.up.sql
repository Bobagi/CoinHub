BEGIN;

-- Coarse geolocation resolved from the access IP at record time (via MaxMind GeoLite2-City), so the
-- access history can show "where" a sign-in came from, like analytics tools do. Nullable: older rows
-- and accesses where the IP can't be resolved (or the DB isn't provisioned) simply have no location.
ALTER TABLE account_access_events ADD COLUMN IF NOT EXISTS country_code VARCHAR(2);
ALTER TABLE account_access_events ADD COLUMN IF NOT EXISTS country_name VARCHAR(80);
ALTER TABLE account_access_events ADD COLUMN IF NOT EXISTS region VARCHAR(120);
ALTER TABLE account_access_events ADD COLUMN IF NOT EXISTS city VARCHAR(120);

COMMIT;
