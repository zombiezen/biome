create table "local_files" (
  "biome_id" text
    not null
    references "biomes"
      on update cascade
      on delete cascade,
  "path" text
    not null
    check ("path" <> ''),
  "stamp" text
    not null
    default '0',

  primary key ("biome_id", "path")
);
