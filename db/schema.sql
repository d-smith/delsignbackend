create table users (
    email text primary key,
    pubkey text not null,
    created_at datetime default current_timestamp
);

create table wallets (
    id integer primary key autoincrement,
    email text not null,
    created_at datetime default current_timestamp,
    foreign key (email) references users (email)
);