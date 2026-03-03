# Dự án FlashQueue

func TransferMoney(ctx context.Context, db *sqlx.DB, fromID, toID int, amount float64) error {
    tx,err := db.beginTxx(ctx,nil)
    if err != nil{
        return err
    }
}