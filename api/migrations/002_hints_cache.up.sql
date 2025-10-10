create table if not exists hints_cache (
                                           image_hash text not null,
                                           engine text not null,
                                           model text not null,
                                           level int not null, -- 1..3
                                           hint_json jsonb not null,
                                           created_at timestamptz not null default now(),
                                           primary key (image_hash, engine, model, level)
);