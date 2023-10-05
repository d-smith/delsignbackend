create table users (
    email text primary key,
    pubkey text not null,
    created_at datetime default current_timestamp
);