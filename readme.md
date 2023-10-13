# Delsignbackend - Delegated Signing Backend

Simple backend for delegated signing example. 


## Sqlite3 DB  

For simple PoC work this version uses a sqlite3 database.

To create the database:

```bash
sqlite3 delsign.db < db/schema.sql
```

To interact with the database:

```
sqlite3 delsign.db
```

Some useful SQL:

First, seed a wallet with a preseeded ganache account:

```
insert into addresses(wallet_id, address, private_key, public_key) values(104,'0x73dA1eD554De26C467d97ADE090af6d52851745E','0xf9832eeac47db42efeb2eca01e6479bfde00fda8fdd0624d45efd0e4b9ddcd3b','0x04155e7dc15dddd66be1beb6d735d03e65290642450ed2b38f676aa4943c19c0f35da28d7e7198fceeea2367f67c75b608077f63bbf8b9376f192269e602830278')
```

And in the absence of an address book, grab other wallet addresses for the current user:

``` 
select address from addresses where wallet_id in (select id from wallets where email='doug.smith.mail@gmail.com')

0xB7Ed5c8B13176150b9b7A6fA3027Bc4818236Ed5
0xbD9604320bfcDabB5F6FBA760d01BbaDb989d229
0x3EDE0E50a0979DAc615Ee9c7C38545d15cd582a0
0xD3Cf3Bfa79b09E091631C5F6930fB14F7Cb68db5
0x0289E2030DBFEE0FDD4AC1A5b1Dd474B13Ac599E
0x73dA1eD554De26C467d97ADE090af6d52851745E
```