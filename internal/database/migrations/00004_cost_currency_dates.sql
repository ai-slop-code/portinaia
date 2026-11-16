-- +goose Up
-- ISO 4217 List One, maintained by SIX, published 2026-09-17.
CREATE TABLE currency_units (
    currency_code TEXT PRIMARY KEY CHECK (length(currency_code) = 3 AND currency_code = upper(currency_code) AND currency_code NOT GLOB '*[^A-Z]*'),
    minor_unit_exponent INTEGER NOT NULL CHECK (minor_unit_exponent BETWEEN 0 AND 4),
    UNIQUE (currency_code, minor_unit_exponent)
);

INSERT INTO currency_units (currency_code, minor_unit_exponent) VALUES
    ('AED', 2), ('AFN', 2), ('ALL', 2), ('AMD', 2), ('AOA', 2), ('ARS', 2),
    ('AUD', 2), ('AWG', 2), ('AZN', 2), ('BAM', 2), ('BBD', 2), ('BDT', 2),
    ('BHD', 3), ('BIF', 0), ('BMD', 2), ('BND', 2), ('BOB', 2), ('BOV', 2),
    ('BRL', 2), ('BSD', 2), ('BTN', 2), ('BWP', 2), ('BYN', 2), ('BZD', 2),
    ('CAD', 2), ('CDF', 2), ('CHE', 2), ('CHF', 2), ('CHW', 2), ('CLF', 4),
    ('CLP', 0), ('CNY', 2), ('COP', 2), ('COU', 2), ('CRC', 2), ('CUP', 2),
    ('CVE', 2), ('CZK', 2), ('DJF', 0), ('DKK', 2), ('DOP', 2), ('DZD', 2),
    ('EGP', 2), ('ERN', 2), ('ETB', 2), ('EUR', 2), ('FJD', 2), ('FKP', 2),
    ('GBP', 2), ('GEL', 2), ('GHS', 2), ('GIP', 2), ('GMD', 2), ('GNF', 0),
    ('GTQ', 2), ('GYD', 2), ('HKD', 2), ('HNL', 2), ('HTG', 2), ('HUF', 2),
    ('IDR', 2), ('ILS', 2), ('INR', 2), ('IQD', 3), ('IRR', 2), ('ISK', 0),
    ('JMD', 2), ('JOD', 3), ('JPY', 0), ('KES', 2), ('KGS', 2), ('KHR', 2),
    ('KMF', 0), ('KPW', 2), ('KRW', 0), ('KWD', 3), ('KYD', 2), ('KZT', 2),
    ('LAK', 2), ('LBP', 2), ('LKR', 2), ('LRD', 2), ('LSL', 2), ('LYD', 3),
    ('MAD', 2), ('MDL', 2), ('MGA', 2), ('MKD', 2), ('MMK', 2), ('MNT', 2),
    ('MOP', 2), ('MRU', 2), ('MUR', 2), ('MVR', 2), ('MWK', 2), ('MXN', 2),
    ('MXV', 2), ('MYR', 2), ('MZN', 2), ('NAD', 2), ('NGN', 2), ('NIO', 2),
    ('NOK', 2), ('NPR', 2), ('NZD', 2), ('OMR', 3), ('PAB', 2), ('PEN', 2),
    ('PGK', 2), ('PHP', 2), ('PKR', 2), ('PLN', 2), ('PYG', 0), ('QAR', 2),
    ('RON', 2), ('RSD', 2), ('RUB', 2), ('RWF', 0), ('SAR', 2), ('SBD', 2),
    ('SCR', 2), ('SDG', 2), ('SEK', 2), ('SGD', 2), ('SHP', 2), ('SLE', 2),
    ('SOS', 2), ('SRD', 2), ('SSP', 2), ('STN', 2), ('SVC', 2), ('SYP', 2),
    ('SZL', 2), ('THB', 2), ('TJS', 2), ('TMT', 2), ('TND', 3), ('TOP', 2),
    ('TRY', 2), ('TTD', 2), ('TWD', 2), ('TZS', 2), ('UAH', 2), ('UGX', 0),
    ('USD', 2), ('USN', 2), ('UYI', 0), ('UYU', 2), ('UYW', 4), ('UZS', 2),
    ('VED', 2), ('VES', 2), ('VND', 0), ('VUV', 0), ('WST', 2), ('XAD', 2),
    ('XAF', 0), ('XCD', 2), ('XCG', 2), ('XOF', 0), ('XPF', 0), ('YER', 2),
    ('ZAR', 2), ('ZMW', 2), ('ZWG', 2);

CREATE TABLE _wp02_costs_source AS SELECT * FROM costs;
CREATE TABLE costs_new (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    compute_id TEXT REFERENCES computes(id) ON DELETE RESTRICT,
    domain_id TEXT REFERENCES domains(id) ON DELETE RESTRICT,
    type TEXT NOT NULL CHECK (type IN ('recurring', 'one_time')),
    amount_minor INTEGER NOT NULL CHECK (amount_minor >= 0),
    currency_code TEXT NOT NULL,
    currency_exponent INTEGER NOT NULL CHECK (currency_exponent BETWEEN 0 AND 4),
    billing_interval TEXT CHECK (billing_interval IS NULL OR billing_interval IN ('monthly', 'quarterly', 'semiannual', 'annual')),
    effective_start_date TEXT CHECK (effective_start_date IS NULL OR (effective_start_date GLOB '????-??-??' AND date(effective_start_date) = effective_start_date)),
    effective_end_date TEXT CHECK (effective_end_date IS NULL OR (effective_end_date GLOB '????-??-??' AND date(effective_end_date) = effective_end_date)),
    charge_date TEXT CHECK (charge_date IS NULL OR (charge_date GLOB '????-??-??' AND date(charge_date) = charge_date)),
    description TEXT NOT NULL DEFAULT '',
    provider_id TEXT REFERENCES providers(id) ON DELETE RESTRICT,
    provider_reference TEXT,
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    updated_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', updated_at) = updated_at, 0)),
    archived_at TEXT CHECK (archived_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', archived_at) = archived_at, 0)),
    FOREIGN KEY (currency_code, currency_exponent) REFERENCES currency_units(currency_code, minor_unit_exponent) ON UPDATE RESTRICT ON DELETE RESTRICT,
    CHECK ((compute_id IS NOT NULL) + (domain_id IS NOT NULL) = 1),
    CHECK ((type = 'recurring' AND billing_interval IS NOT NULL AND effective_start_date IS NOT NULL AND charge_date IS NULL)
        OR (type = 'one_time' AND billing_interval IS NULL AND effective_start_date IS NULL AND effective_end_date IS NULL AND charge_date IS NOT NULL)),
    CHECK (effective_end_date IS NULL OR (effective_start_date IS NOT NULL AND effective_end_date >= effective_start_date)),
    CHECK (updated_at >= created_at)
);

-- One-time schema-1 rows had an unrelated required effective start date. The approved one-time model
-- has only charge_date, so the obsolete effective start/end fields are explicitly cleared on copy.
INSERT INTO costs_new (
    id, compute_id, domain_id, type, amount_minor, currency_code, currency_exponent,
    billing_interval, effective_start_date, effective_end_date, charge_date,
    description, provider_id, provider_reference, revision, created_at, updated_at, archived_at
)
SELECT id, compute_id, domain_id, type, amount_minor, currency_code, currency_exponent,
    billing_interval,
    CASE WHEN type = 'recurring' THEN effective_start_date ELSE NULL END,
    CASE WHEN type = 'recurring' THEN effective_end_date ELSE NULL END,
    charge_date, description, provider_id, provider_reference, revision, created_at, updated_at, archived_at
FROM _wp02_costs_source;

DROP TABLE costs;
ALTER TABLE costs_new RENAME TO costs;
DROP TABLE _wp02_costs_source;

CREATE INDEX costs_compute_idx ON costs (compute_id, archived_at, effective_start_date DESC);
CREATE INDEX costs_domain_idx ON costs (domain_id, archived_at, effective_start_date DESC);
CREATE INDEX costs_recurring_idx ON costs (currency_code, billing_interval, effective_start_date) WHERE type = 'recurring' AND archived_at IS NULL;

PRAGMA user_version = 4;

-- +goose Down
CREATE TABLE _wp02_down_guard (valid INTEGER NOT NULL CHECK (valid = 1));
INSERT INTO _wp02_down_guard (valid)
SELECT CASE WHEN EXISTS (SELECT 1 FROM costs) THEN 0 ELSE 1 END;
DROP TABLE _wp02_down_guard;

DROP TABLE costs;
CREATE TABLE costs (
    id TEXT PRIMARY KEY CHECK (length(id) = 36 AND id = lower(id) AND length(replace(id, '-', '')) = 32
        AND substr(id, 9, 1) = '-' AND substr(id, 14, 1) = '-' AND substr(id, 19, 1) = '-' AND substr(id, 24, 1) = '-'
        AND replace(id, '-', '') NOT GLOB '*[^0-9a-f]*'),
    compute_id TEXT REFERENCES computes(id) ON DELETE RESTRICT,
    domain_id TEXT REFERENCES domains(id) ON DELETE RESTRICT,
    type TEXT NOT NULL CHECK (type IN ('recurring', 'one_time')),
    amount_minor INTEGER NOT NULL CHECK (amount_minor >= 0),
    currency_code TEXT NOT NULL CHECK (length(currency_code) = 3 AND currency_code = upper(currency_code) AND currency_code NOT GLOB '*[^A-Z]*'),
    currency_exponent INTEGER NOT NULL CHECK (currency_exponent BETWEEN 0 AND 3),
    billing_interval TEXT CHECK (billing_interval IS NULL OR billing_interval IN ('monthly', 'quarterly', 'semiannual', 'annual')),
    effective_start_date TEXT NOT NULL CHECK (effective_start_date GLOB '????-??-??' AND date(effective_start_date) = effective_start_date),
    effective_end_date TEXT CHECK (effective_end_date IS NULL OR (effective_end_date GLOB '????-??-??' AND date(effective_end_date) = effective_end_date)),
    charge_date TEXT CHECK (charge_date IS NULL OR (charge_date GLOB '????-??-??' AND date(charge_date) = charge_date)),
    description TEXT NOT NULL DEFAULT '',
    provider_id TEXT REFERENCES providers(id) ON DELETE RESTRICT,
    provider_reference TEXT,
    revision INTEGER NOT NULL DEFAULT 1 CHECK (revision >= 1),
    created_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', created_at) = created_at, 0)),
    updated_at TEXT NOT NULL CHECK (COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', updated_at) = updated_at, 0)),
    archived_at TEXT CHECK (archived_at IS NULL OR COALESCE(strftime('%Y-%m-%dT%H:%M:%SZ', archived_at) = archived_at, 0)),
    CHECK ((compute_id IS NOT NULL) + (domain_id IS NOT NULL) = 1),
    CHECK ((type = 'recurring' AND billing_interval IS NOT NULL AND charge_date IS NULL)
        OR (type = 'one_time' AND billing_interval IS NULL AND charge_date IS NOT NULL)),
    CHECK (effective_end_date IS NULL OR effective_end_date >= effective_start_date),
    CHECK (updated_at >= created_at)
);
CREATE INDEX costs_compute_idx ON costs (compute_id, archived_at, effective_start_date DESC);
CREATE INDEX costs_domain_idx ON costs (domain_id, archived_at, effective_start_date DESC);
CREATE INDEX costs_recurring_idx ON costs (currency_code, billing_interval, effective_start_date) WHERE type = 'recurring' AND archived_at IS NULL;
DROP TABLE currency_units;
PRAGMA user_version = 3;
