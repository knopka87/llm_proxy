create table if not exists parsed_tasks (
                                            id bigserial primary key,
                                            created_at timestamptz not null default now(),
                                            chat_id bigint,
                                            media_group_id text,
                                            image_hash text not null,
                                            engine text not null,
                                            model text not null,
                                            grade_hint int,
                                            subject_hint text,
                                            raw_text text not null,
                                            question text not null,
                                            result_json jsonb not null,
                                            confidence numeric,
                                            confirmation_needed boolean not null,
                                            unique (image_hash, engine, model)
);
CREATE INDEX IF NOT EXISTS idx_parsed_tasks_chat_time ON parsed_tasks (chat_id, created_at DESC);