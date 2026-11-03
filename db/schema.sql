create table if not exists users (
    email text primary key,
    pubkey text not null,
    created_at datetime default current_timestamp
);

create table if not exists wallets (
    id integer primary key autoincrement,
    email text not null,
    created_at datetime default current_timestamp,
    foreign key (email) references users (email)
);

create table if not exists addresses (
    id integer primary key autoincrement,
    wallet_id integer not null,
    address text not null,
    private_key text not null,
    public_key text not null,
    created_at datetime default current_timestamp,
    foreign key (wallet_id) references wallets (id)
);

-- Request ids of accepted send requests, kept until the request's expiry
-- passes, so a signed request can't be replayed.
create table if not exists used_request_ids (
    email text not null,
    request_id text not null,
    expiry integer not null,
    primary key (email, request_id)
);
