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
    private_key test not null,
    public_key text not null,
    created_at datetime default current_timestamp,
    foreign key (wallet_id) references wallets (id)
);