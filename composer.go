package tusd

type StoreComposer struct {
	Core DataStore

	UsesTerminater bool
	Terminater     TerminaterDataStore
	UsesFinisher   bool
	Finisher       FinisherDataStore
	UsesLocker     bool
	Locker         LockerDataStore
	UsesGetReader  bool
	GetReader      GetReaderDataStore
	UsesConcater   bool
	Concater       ConcaterDataStore
}

func NewStoreComposer() *StoreComposer {
	return &StoreComposer{}
}

func NewStoreComposerFromDataStore(store DataStore) *StoreComposer {
	composer := NewStoreComposer()
	composer.UseCore(store)

	if mod, ok := store.(TerminaterDataStore); ok {
		composer.UseTerminater(mod)
	}
	if mod, ok := store.(FinisherDataStore); ok {
		composer.UseFinisher(mod)
	}
	if mod, ok := store.(LockerDataStore); ok {
		composer.UseLocker(mod)
	}
	if mod, ok := store.(GetReaderDataStore); ok {
		composer.UseGetReader(mod)
	}
	if mod, ok := store.(ConcaterDataStore); ok {
		composer.UseConcater(mod)
	}

	return composer
}

func (store *StoreComposer) UseCore(core DataStore) {
	store.Core = core
}

func (store *StoreComposer) UseTerminater(x TerminaterDataStore) {
	store.UsesTerminater = x != nil
	store.Terminater = x
}
func (store *StoreComposer) UseFinisher(x FinisherDataStore) {
	store.UsesFinisher = x != nil
	store.Finisher = x
}
func (store *StoreComposer) UseLocker(x LockerDataStore) {
	store.UsesLocker = x != nil
	store.Locker = x
}
func (store *StoreComposer) UseGetReader(x GetReaderDataStore) {
	store.UsesGetReader = x != nil
	store.GetReader = x
}
func (store *StoreComposer) UseConcater(x ConcaterDataStore) {
	store.UsesConcater = x != nil
	store.Concater = x
}
