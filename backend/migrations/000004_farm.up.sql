-- farm module: fields, soil profiles, plots. owner_id references identity by
-- value only (no cross-module foreign key).
CREATE TABLE farm_fields (
    id          uuid PRIMARY KEY,
    owner_id    uuid        NOT NULL,
    name        text        NOT NULL,
    area_nano   bigint      NOT NULL CHECK (area_nano > 0),
    area_unit   text        NOT NULL,
    lat_e7      bigint      NOT NULL,
    lng_e7      bigint      NOT NULL,
    district    text        NOT NULL DEFAULT '',
    irrigation  text        NOT NULL,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL
);
CREATE INDEX farm_fields_owner_idx ON farm_fields (owner_id, created_at);

CREATE TABLE farm_soil_profiles (
    field_id           uuid PRIMARY KEY REFERENCES farm_fields (id) ON DELETE CASCADE,
    texture            text    NOT NULL,
    ph_x10             integer NOT NULL,
    nitrogen_kg_ha     integer NOT NULL,
    phosphorus_kg_ha   integer NOT NULL,
    potassium_kg_ha    integer NOT NULL,
    organic_matter_x10 integer NOT NULL,
    tested_on          date
);

CREATE TABLE farm_plots (
    id         uuid PRIMARY KEY,
    field_id   uuid        NOT NULL REFERENCES farm_fields (id) ON DELETE CASCADE,
    name       text        NOT NULL,
    area_nano  bigint      NOT NULL CHECK (area_nano > 0),
    crop_code  text        NOT NULL,
    sown_on    date,
    created_at timestamptz NOT NULL
);
CREATE INDEX farm_plots_field_idx ON farm_plots (field_id);
