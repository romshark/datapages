package main

import (
	"context"
	"datapages/app/domain"
	"time"
)

func timestamp(value string) time.Time {
	v, err := time.Parse(time.RFC3339, value)
	if err != nil {
		panic(err)
	}
	return v
}

var mainCategories = []domain.Category{
	{
		ID: "cars", Name: "Cars",
		ImageURL: "/static/icons/category-cars.png",
	},
	{
		ID: "musical-instruments", Name: "Musical Instruments",
		ImageURL: "/static/icons/category-musical-instruments.png",
	},
	{
		ID: "clothing", Name: "Clothing",
		ImageURL: "/static/icons/category-clothing.png",
	},
	{
		ID: "electronics", Name: "Electronics",
		ImageURL: "/static/icons/category-electronics.png",
	},
}

func NewRepository() *domain.Repository {
	ctx := context.Background()
	slugNonceGen := domain.NewSeededSlugNonceGenerator(1, 2)
	repo := domain.NewRepository(mainCategories, slugNonceGen)

	users := map[string]domain.User{
		"testuser": {
			PasswordHash:   "testuser",
			AccountCreated: timestamp("2023-10-10T10:10:10Z"),
			Email:          "user@test.net",
		},
		"autohandel-maerz-regensburg": {
			PasswordHash:   "pass-autohandel",
			AccountCreated: timestamp("2024-02-10T09:15:00Z"),
			Email:          "autohandel@autohaus-fake.de",
		},
		"julianf92": {
			PasswordHash:   "julian123",
			AccountCreated: timestamp("2024-03-05T14:22:00Z"),
			Email:          "julianf92@mailbox.test",
		},
		"fabiberg": {
			PasswordHash:   "fabipass",
			AccountCreated: timestamp("2024-04-18T08:00:00Z"),
			Email:          "fabiberg@users.fake",
		},
		"moritz-keilmann": {
			PasswordHash:   "moritzpw",
			AccountCreated: timestamp("2024-06-01T19:45:00Z"),
			Email:          "moritz.keilmann@post.example",
		},
		"johaness2442": {
			PasswordHash:   "johannespw",
			AccountCreated: timestamp("2024-07-12T11:30:00Z"),
			Email:          "johaness2442@inbox.fake",
		},
		"karl-heinz-schmidt": {
			PasswordHash:   "karlheinz",
			AccountCreated: timestamp("2024-08-03T16:10:00Z"),
			Email:          "karl.schmidt@private.test",
		},
		"peter-lobert-mayer": {
			PasswordHash:   "petermayer",
			AccountCreated: timestamp("2024-09-20T07:55:00Z"),
			Email:          "peter.mayer@secure.fake",
		},
		"osna2": {
			PasswordHash:   "osnaosna",
			AccountCreated: timestamp("2024-10-11T21:40:00Z"),
			Email:          "osna2@chat.test",
		},
		"gretschen": {
			PasswordHash:   "gretschpw",
			AccountCreated: timestamp("2024-11-02T10:05:00Z"),
			Email:          "gretschen@people.fake",
		},
		"hans-schwab90": {
			PasswordHash:   "hansschwab",
			AccountCreated: timestamp("2024-12-15T18:25:00Z"),
			Email:          "hans.schwab90@users.test",
		},
		"patricia-haas2": {
			PasswordHash:   "patriciahaas",
			AccountCreated: timestamp("2025-01-09T13:00:00Z"),
			Email:          "patricia.haas@contacts.fake",
		},
		"kaiy": {
			PasswordHash:   "kaiypass1",
			AccountCreated: timestamp("2025-02-14T09:35:00Z"),
			Email:          "kaiy@alpha.test",
		},
		"KFCFan": {
			PasswordHash:   "kfcfan123",
			AccountCreated: timestamp("2025-03-01T17:50:00Z"),
			Email:          "kfcfan@fake.test",
		},
		"gehlert_gmbh": {
			PasswordHash:   "gehlertgmbh",
			AccountCreated: timestamp("2025-04-22T06:45:00Z"),
			Email:          "info@gehlert-gmbh.fake",
		},
		"lorentz553": {
			PasswordHash:   "lorentzpw",
			AccountCreated: timestamp("2025-05-30T12:20:00Z"),
			Email:          "lorentz553@members.test",
		},
		"margarita-lehmann": {
			PasswordHash:   "margaritapw",
			AccountCreated: timestamp("2025-06-18T15:10:00Z"),
			Email:          "margarita.lehmann@profiles.fake",
		},
		"jjk-331": {
			PasswordHash:   "jjk331pw",
			AccountCreated: timestamp("2025-07-07T08:40:00Z"),
			Email:          "jjk331@gaming.test",
		},
		"laura-freimann66224": {
			PasswordHash:   "laurapw",
			AccountCreated: timestamp("2025-08-19T20:00:00Z"),
			Email:          "laura.freimann@social.fake",
		},
		"ines-schwarz": {
			PasswordHash:   "inesschwarz",
			AccountCreated: timestamp("2025-09-03T11:55:00Z"),
			Email:          "ines.schwarz@work.test",
		},
		"hoa-nguyen": {
			PasswordHash:   "hoanguyen",
			AccountCreated: timestamp("2025-10-14T07:10:00Z"),
			Email:          "hoa.nguyen@asia.fake",
		},
		"tilll": {
			PasswordHash:   "tilllpw",
			AccountCreated: timestamp("2025-11-01T16:30:00Z"),
			Email:          "tilll@shortname.test",
		},
		"paola-fra": {
			PasswordHash:   "paolapw",
			AccountCreated: timestamp("2025-11-20T09:00:00Z"),
			Email:          "paola.fra@eu.fake",
		},
		"pipboy42": {
			PasswordHash:   "pipboypw",
			AccountCreated: timestamp("2025-12-05T22:15:00Z"),
			Email:          "pipboy42@games.test",
		},
		"kerzer-j": {
			PasswordHash:   "kerzerpw",
			AccountCreated: timestamp("2025-12-20T05:35:00Z"),
			Email:          "kerzer.j@tools.fake",
		},
		"maroni": {
			PasswordHash:   "maronipw",
			AccountCreated: timestamp("2025-12-31T23:59:00Z"),
			Email:          "maroni@lastday.test",
		},
	}
	for displayName, u := range users {
		id, err := repo.NewUser(
			ctx, displayName, u.AvatarImageURL, u.Email, u.PasswordHash,
		)
		if err != nil {
			panic(err)
		}
		newUser, err := repo.UserByID(ctx, id)
		if err != nil {
			panic(err)
		}
		users[displayName] = newUser
	}

	// https://picsum.photos/400/300
	posts := []domain.Post{
		// category: Cars
		{
			ID:             "1001",
			CategoryID:     "cars",
			ImageURL:       "https://images.pexels.com/photos/810357/pexels-photo-810357.jpeg",
			MerchantUserID: "autohandel-maerz-regensburg",
			Title:          "Mercedes-Benz A-Class Hatchback - Blue, Sporty and Well Kept",
			Description:    "Used Mercedes-Benz A-Class in blue with a clean, modern look. Exterior is in good condition with alloy wheels and no visible accident damage, only normal wear from regular use. Compact yet comfortable, suitable for city driving and longer trips. Sold as shown, no additional extras.",
			Price:          14200,
			TimePosted:     timestamp("2025-11-08T09:45:00Z"),
			Location:       "Regensburg",
		},
		{
			ID:             "1002",
			CategoryID:     "cars",
			ImageURL:       "https://images.pexels.com/photos/100656/pexels-photo-100656.jpeg",
			MerchantUserID: "autohandel-maerz-regensburg",
			Title:          "BMW 3 Series Sedan - White, Clean and Well Maintained",
			Description:    "Used BMW 3 Series sedan in white with a classic, elegant look. Vehicle appears well cared for with clean body lines, alloy wheels, and no visible accident damage. Comfortable and solid driving experience, suitable for daily commuting and longer trips. Normal signs of use only. Sold as shown.",
			Price:          16800,
			TimePosted:     timestamp("2025-11-03T16:20:00Z"),
			Location:       "Regensburg",
		},
		{
			ID:             "1003",
			CategoryID:     "cars",
			ImageURL:       "https://images.pexels.com/photos/8408981/pexels-photo-8408981.png",
			MerchantUserID: "julianf92",
			Title:          "Volkswagen Golf II - Classic Hatchback, Orange",
			Description:    "Classic VW Golf II in distinctive orange color. Honest, used condition with visible signs of age and use, but overall complete and original look. Ideal as a daily classic, project car, or for enthusiasts who appreciate old-school Volkswagen styling. Exterior shows patina consistent with its age. Sold as seen.",
			Price:          4200,
			TimePosted:     timestamp("2025-10-12T11:40:00Z"),
			Location:       "Bochum",
		},
		{
			ID:             "1004",
			CategoryID:     "cars",
			ImageURL:       "https://images.pexels.com/photos/6238437/pexels-photo-6238437.jpeg",
			MerchantUserID: "fabiberg",
			Title:          "Toyota GT86 Coupe - White, Sporty and Clean",
			Description:    "White with a sporty, low-profile design. Vehicle appears well maintained with clean bodywork, modern headlights, and alloy wheels. Fun rear-wheel-drive car with a focused driving feel, suitable for enthusiasts and daily use alike. Only normal signs of use visible. Sold as shown.",
			Price:          18500,
			TimePosted:     timestamp("2025-11-18T13:10:00Z"),
			Location:       "Kassel",
		},
		{
			ID:             "1005",
			CategoryID:     "cars",
			ImageURL:       "https://images.pexels.com/photos/175568/pexels-photo-175568.jpeg",
			MerchantUserID: "moritz-keilmann",
			Title:          "Vintage 1920s Classic Car - Fully Restored, Museum Quality",
			Description:    "Authentic 1920s-era classic automobile in exceptional restored condition. Features period-correct details, chrome accents, original-style wheels, and a beautifully preserved interior. Ideal for collectors, exhibitions, or vintage car enthusiasts. Vehicle shows outstanding craftsmanship and care, suitable for shows or private collections. Serious inquiries only.",
			Price:          78000,
			TimePosted:     timestamp("2025-10-05T12:00:00Z"),
			Location:       "Heidelberg",
		},
		{
			ID:             "1006",
			CategoryID:     "cars",
			ImageURL:       "https://images.pexels.com/photos/757181/pexels-photo-757181.jpeg",
			MerchantUserID: "johaness2442",
			Title:          "Volkswagen T2 Camper Van - Red/White, Classic Look",
			Description:    "Classic VW T2 bus in red and white with iconic front design. Used condition with visible patina that matches its age, giving it an authentic vintage character. Suitable as a hobby vehicle, camper base, or collector's piece. Complete exterior, original styling, and lots of charm. Sold as seen.",
			Price:          26500,
			TimePosted:     timestamp("2025-10-20T15:05:00Z"),
			Location:       "Oldenburg",
		},

		// category: Musical Instruments
		{
			ID:             "2008",
			CategoryID:     "musical-instruments",
			ImageURL:       "https://images.pexels.com/photos/1010519/pexels-photo-1010519.jpeg",
			MerchantUserID: "karl-heinz-schmidt",
			Title:          "Acoustic Guitar - Natural Wood, Classic Design",
			Description:    "Steel-string acoustic guitar in natural wood finish with a timeless, simple look. Used condition with light signs of wear, but overall well kept and fully functional. Suitable for beginners and intermediate players, ideal for home playing, practice, or casual performances. Warm sound and comfortable to play. Sold as seen.",
			Price:          280,
			TimePosted:     timestamp("2025-11-12T18:25:00Z"),
			Location:       "Hildesheim",
		},
		{
			ID:             "2009",
			CategoryID:     "musical-instruments",
			ImageURL:       "https://images.pexels.com/photos/2043571/pexels-photo-2043571.jpeg",
			MerchantUserID: "peter-lobert-mayer",
			Title:          "Baby Grand Piano - Polished Wood, Elegant Condition",
			Description:    "Beautiful baby grand piano with a polished wood finish and classic design. Used but well cared for, with clean keys, intact mechanics, and a rich, warm sound. Ideal for home use, teaching, or small performances. Shows only minor signs of age consistent with careful ownership. Bench included. Sold as seen.",
			Price:          26900,
			TimePosted:     timestamp("2025-11-06T10:55:00Z"),
			Location:       "Weimar",
		},
		{
			ID:             "20010",
			CategoryID:     "musical-instruments",
			ImageURL:       "https://images.pexels.com/photos/32218646/pexels-photo-32218646.jpeg",
			MerchantUserID: "peter-lobert-mayer",
			Title:          "Weber Upright Piano - Classic Wood Finish with Bench",
			Description:    "Upright piano by Weber in a classic wooden finish. Used and well preserved, with intact keys and a warm, balanced sound. Ideal for home use, practice, or teaching. Shows light cosmetic wear consistent with age but fully functional. Includes matching piano bench. Pickup required. Sold as seen.",
			Price:          3200,
			TimePosted:     timestamp("2025-10-28T17:35:00Z"),
			Location:       "Göttingen",
		},
		{
			ID:             "20011",
			CategoryID:     "musical-instruments",
			ImageURL:       "https://images.pexels.com/photos/164936/pexels-photo-164936.jpeg",
			MerchantUserID: "osna2",
			Title:          "Tenor Saxophone - Brass Finish with Case",
			Description:    "Polished brass finish, complete with soft case. Used but well maintained, all keys and pads appear intact and responsive. Suitable for intermediate to advanced players, rehearsals, or live performances. Shows light cosmetic wear consistent with normal use. Ready to play. Sold as seen.",
			Price:          1200,
			TimePosted:     timestamp("2025-11-09T14:40:00Z"),
			Location:       "Osnabrück",
		},
		// category: Clothing
		{
			ID:             "30012",
			CategoryID:     "clothing",
			ImageURL:       "https://images.pexels.com/photos/2356344/pexels-photo-2356344.jpeg",
			MerchantUserID: "gretschen",
			Title:          "Vintage-Style T-Shirt - Yellow with White Trim",
			Description:    "Bright yellow vintage-style T-shirt with white collar and sleeve trim. Clean, well-kept condition with no visible stains or damage. Soft fabric, comfortable fit, and classic casual look. Suitable for everyday wear or as a retro-inspired piece. Sold as shown.",
			Price:          25,
			TimePosted:     timestamp("2025-11-14T16:10:00Z"),
			Location:       "Augsburg",
		},
		{
			ID:             "30013",
			CategoryID:     "clothing",
			ImageURL:       "https://images.pexels.com/photos/2464090/pexels-photo-2464090.jpeg",
			MerchantUserID: "hans-schwab90",
			Title:          "Graphic T-Shirt - Grey with Back Print Text",
			Description:    "With large printed text on the back and minimal branding on the sleeve.",
			Price:          18,
			TimePosted:     timestamp("2025-11-16T12:30:00Z"),
			Location:       "Paderborn",
		},
		{
			ID:             "30014",
			CategoryID:     "clothing",
			ImageURL:       "https://images.pexels.com/photos/13094233/pexels-photo-13094233.jpeg",
			MerchantUserID: "patricia-haas2",
			Title:          "Grey Wool Coat - Minimal Design, Clean Condition",
			Description:    "Clean, minimal design and classic button closure. Used but well maintained, fabric feels solid with no visible damage or stains. Suitable for everyday wear, office use, or casual outfits during colder seasons. Timeless style that pairs easily with most wardrobes. Sold as shown.",
			Price:          120,
			TimePosted:     timestamp("2025-11-21T09:20:00Z"),
			Location:       "Freiburg",
		},
		// category: Electronics
		{
			ID:             "40015",
			CategoryID:     "electronics",
			ImageURL:       "https://images.pexels.com/photos/32075199/pexels-photo-32075199.jpeg",
			MerchantUserID: "kaiy",
			Title:          "Nintendo Game Boy Advance SP - NES Edition Style",
			Description:    "Game Boy in a classic NES-inspired design. Used condition with visible signs of use, but fully functional. Screen powers on correctly, buttons responsive, and hinges intact. Ideal for retro gaming fans or collectors. Console only, no games or charger included. Sold as seen.",
			Price:          110,
			TimePosted:     timestamp("2025-11-19T11:05:00Z"),
			Location:       "Bamberg",
		},
		{
			ID:             "40016",
			CategoryID:     "electronics",
			ImageURL:       "https://images.pexels.com/photos/5961216/pexels-photo-5961216.jpeg",
			MerchantUserID: "kaiy",
			Title:          "Sony PlayStation 5 Console - Disc Version with Controller",
			Description:    "Sony PlayStation 5 - clean, well-kept condition. Includes original DualSense controller. Console shows only light signs of use, fully functional and ready to play. Ideal for current-gen gaming with fast load times and smooth performance. No games included. Sold as shown.",
			Price:          420,
			TimePosted:     timestamp("2025-11-24T19:10:00Z"),
			Location:       "Ulm",
		},
		{
			ID:             "40017",
			CategoryID:     "electronics",
			ImageURL:       "https://images.pexels.com/photos/17941534/pexels-photo-17941534.jpeg",
			MerchantUserID: "gehlert_gmbh",
			Title:          "Sony Walkman FM/AM Cassette Player - With Headphones and Tapes",
			Description:    "Used but good condition. Includes original-style wired headphones and the depicted cassette tapes. Device powers on and appears complete, with normal signs of age and use. Ideal for retro audio fans or collectors looking for an authentic 90s listening experience. Sold as a set, exactly as shown.",
			Price:          95,
			TimePosted:     timestamp("2025-11-17T10:50:00Z"),
			Location:       "Koblenz",
		},
		{
			ID:             "40018",
			CategoryID:     "electronics",
			ImageURL:       "https://images.pexels.com/photos/19938246/pexels-photo-19938246.jpeg",
			MerchantUserID: "gehlert_gmbh",
			Title:          "Sony Portable FM/AM Radio - Compact and Reliable",
			Description:    "Compact Sony FM/AM portable radio in clean used condition.",
			Price:          30,
			TimePosted:     timestamp("2025-11-22T08:40:00Z"),
			Location:       "Marburg",
		},
		{
			ID:             "40019",
			CategoryID:     "electronics",
			ImageURL:       "https://images.pexels.com/photos/32499807/pexels-photo-32499807.jpeg",
			MerchantUserID: "gehlert_gmbh",
			Title:          "Sony Stereo Cassette System - FM/AM Radio with Speakers",
			Description:    "",
			Price:          140,
			TimePosted:     timestamp("2025-11-20T15:30:00Z"),
			Location:       "Siegen",
		},
		{
			ID:             "40020",
			CategoryID:     "electronics",
			ImageURL:       "https://images.pexels.com/photos/16247537/pexels-photo-16247537.jpeg",
			MerchantUserID: "lorentz553",
			Title:          "Apple iPhone Black, Clean Condition",
			Description:    "Apple iPhone in black with a clean display and well-kept body. Used condition with normal signs of use, no visible cracks on screen. Fully functional touchscreen, buttons, and camera. Suitable for everyday use, apps, and photography. Phone only, no charger or accessories included. Sold as shown.",
			Price:          650,
			TimePosted:     timestamp("2025-11-26T14:20:00Z"),
			Location:       "Karlsruhe",
		},
		{
			ID:             "40021",
			CategoryID:     "electronics",
			ImageURL:       "https://images.pexels.com/photos/30479285/pexels-photo-30479285.jpeg",
			MerchantUserID: "margarita-lehmann",
			Title:          "Apple iPhone (Fully Functional)",
			Description:    "Apple iPhone in black, used but in good overall condition.",
			Price:          600,
			TimePosted:     timestamp("2025-11-27T16:45:00Z"),
			Location:       "Tübingen",
		},
		{
			ID:             "40022",
			CategoryID:     "electronics",
			ImageURL:       "https://images.pexels.com/photos/10774603/pexels-photo-10774603.jpeg",
			MerchantUserID: "jjk-331",
			Title:          "Black Android Smartphone",
			Description:    "Used and in good working condition. Screen is clear with no visible cracks, casing shows normal wear from regular use. Google apps preinstalled and functioning, suitable for calls, messaging, browsing, and everyday apps. Ideal as a main phone or backup device. Phone only, no charger included. Sold as shown.",
			Price:          180,
			TimePosted:     timestamp("2025-11-23T13:55:00Z"),
			Location:       "Jena",
		},
		{
			ID:             "40023",
			CategoryID:     "electronics",
			ImageURL:       "https://images.pexels.com/photos/10774603/pexels-photo-10774603.jpeg",
			MerchantUserID: "laura-freimann66224",
			Title:          "Black Android Smartphone",
			Description:    "Used and in good working condition. Screen is clear with no visible cracks, casing shows normal wear from regular use. Google apps preinstalled and functioning, suitable for calls, messaging, browsing, and everyday apps. Ideal as a main phone or backup device. Phone only, no charger included. Sold as shown.",
			Price:          180,
			TimePosted:     timestamp("2025-11-23T13:55:00Z"),
			Location:       "Jena",
		},
		{
			ID:             "40024",
			CategoryID:     "electronics",
			ImageURL:       "https://images.pexels.com/photos/19810744/pexels-photo-19810744.jpeg",
			MerchantUserID: "ines-schwarz",
			Title:          "Rechargeable AA Batteries with EU Plug Charger",
			Description:    "Set of rechargeable AA batteries including matching wall charger with EU plug. Used but clean and complete, suitable for everyday devices like remotes, radios, toys, or cameras. Practical and cost-saving alternative to disposable batteries. Charger and four batteries included as shown.",
			Price:          15,
			TimePosted:     timestamp("2025-11-25T09:35:00Z"),
			Location:       "Erfurt",
		},
		{
			ID:             "40025",
			CategoryID:     "electronics",
			ImageURL:       "https://images.pexels.com/photos/11031423/pexels-photo-11031423.png",
			MerchantUserID: "hoa-nguyen",
			Title:          "power bank with charging cable",
			Description:    "Compact white power bank in clean used condition. 82% Capacity.",
			Price:          20,
			TimePosted:     timestamp("2025-11-28T10:15:00Z"),
			Location:       "Lüneburg",
		},
		{
			ID:             "40026",
			CategoryID:     "electronics",
			ImageURL:       "https://images.pexels.com/photos/821749/pexels-photo-821749.jpeg",
			MerchantUserID: "tilll",
			Title:          "Professional Camera Equipment Bundle - Cameras, Lenses, Tripod and Accessories",
			Description:    "Complete professional camera kit sold as one set. Includes multiple camera bodies, several interchangeable lenses, external flash unit, sturdy Manfrotto tripod, camera bags, batteries, memory cards, and assorted accessories. Lenses cover different focal lengths for wide-angle, standard, and telephoto shooting, suitable for portraits, landscapes, events, and travel photography. Camera bodies are in used but well-maintained condition with normal cosmetic wear, all controls and dials intact. Tripod is solid and stable, ideal for long exposures, video work, or studio use. Accessories include protective pouches, chargers, cables, and storage boxes as pictured. This is a ready-to-use setup for enthusiasts or professionals looking for a versatile all-in-one photography solution. Sold only as a complete bundle, exactly as shown in the photos.",
			Price:          1850,
			TimePosted:     timestamp("2025-11-29T18:10:00Z"),
			Location:       "Münster",
		},
		{
			ID:             "40027",
			CategoryID:     "electronics",
			ImageURL:       "https://images.pexels.com/photos/1002636/pexels-photo-1002636.jpeg",
			MerchantUserID: "paola-fra",
			Title:          "Manual Focus 28mm Prime Lens (Metal Build), Vintage Optics",
			Description:    "Used manual-focus 28mm prime lens with classic all-metal construction. Fixed focal length of 28mm, ideal for wide-angle photography such as landscapes, street, architecture, and environmental portraits. Aperture range from f/2.8 to f/22, with clearly marked aperture ring featuring full-stop increments for precise exposure control. Manual focus ring operates smoothly, with engraved distance scale in meters and feet, including depth-of-field markings for zone focusing. Lens uses a standard manual bayonet mount (exact mount not specified), suitable for use on compatible film cameras or adapted to mirrorless digital cameras. Optical elements appear clean, with no visible haze or fungus; minor dust possible due to age but does not affect image quality. Compact and lightweight design makes it well suited for travel and everyday shooting. Overall in good used condition with normal cosmetic wear from use. Lens only, no caps or box included. Sold as shown.",
			Price:          180,
			TimePosted:     timestamp("2025-11-30T12:05:00Z"),
			Location:       "Passau",
		},
		{
			ID:             "40028",
			CategoryID:     "electronics",
			ImageURL:       "https://images.pexels.com/photos/279805/pexels-photo-279805.jpeg",
			MerchantUserID: "pipboy42",
			Title:          "Modern Table Lamp Glass Shade",
			Description:    "Simple modern table lamp with round glass shade and metal base. Used, clean condition and fully functional. Provides soft, warm light, ideal for bedroom or living room use. Minimal design fits most interiors. Sold as shown.",
			Price:          35,
			TimePosted:     timestamp("2025-11-18T09:10:00Z"),
			Location:       "Flensburg",
		},
		{
			ID:             "40029",
			CategoryID:     "electronics",
			ImageURL:       "https://images.pexels.com/photos/2566027/pexels-photo-2566027.jpeg",
			MerchantUserID: "kerzer-j",
			Title:          "Commercial Espresso Machine (Dual Group, Stainless Steel)",
			Description:    "Professional-grade espresso machine with dual group heads and stainless steel body. Designed for café or serious home use, featuring steam wand, hot water outlet, cup warmer on top, and intuitive control buttons. Used condition, clean and well maintained, with normal signs of wear from operation. Produces consistent espresso and milk foam. Ideal for coffee bars, restaurants, or experienced home baristas. Sold as shown.",
			Price:          3200,
			TimePosted:     timestamp("2025-11-13T08:55:00Z"),
			Location:       "Konstanz",
		},
		{
			ID:             "40030",
			CategoryID:     "electronics",
			ImageURL:       "https://images.pexels.com/photos/7641488/pexels-photo-7641488.jpeg",
			MerchantUserID: "maroni",
			Title:          " Robot Vacuumer",
			Description:    "Robot vacuum cleaner. Sold as seen. Original receipt and warranty included (1 year left)",
			Price:          170,
			TimePosted:     timestamp("2025-11-21T11:00:00Z"),
			Location:       "Würzburg",
		},
	}
	for i, p := range posts {
		id, err := repo.NewPost(
			ctx, users[p.MerchantUserID].ID, p.Title, p.Description, p.CategoryID,
			p.ImageURL, p.Price, p.Location,
		)
		if err != nil {
			panic(err)
		}
		posts[i].ID = id
	}

	chats := []domain.Chat{
		{
			PostID:       posts[0].ID,
			SenderUserID: users["lorentz553"].ID,
			Messages: []domain.Message{
				{
					Text:         "Hello, is this one still available?",
					SenderUserID: users["lorentz553"].ID,
					TimeSent:     posts[0].TimePosted.Add(2 * time.Minute),
				},
				{
					Text:         "Can you do a 5 percent discount?",
					SenderUserID: users["lorentz553"].ID,
					TimeSent:     posts[0].TimePosted.Add(2*time.Minute + 30*time.Second),
				},
			},
		},
		{
			PostID:       posts[1].ID,
			SenderUserID: users["kaiy"].ID,
			Messages: []domain.Message{
				{
					Text:         "I'd like to buy this",
					SenderUserID: users["kaiy"].ID,
					TimeSent:     posts[1].TimePosted.Add(5 * time.Minute),
				},
				{
					Text:         "Alright, meet me at 6 PM after work!",
					SenderUserID: users[posts[1].MerchantUserID].ID,
					TimeSent:     posts[1].TimePosted.Add(6 * time.Minute),
				},
			},
		},
	}
	for i, c := range chats {
		id, err := repo.NewChat(
			ctx, c.PostID, c.SenderUserID, c.Messages[0].Text,
		)
		if err != nil {
			panic(err)
		}
		posts[i].ID = id

		for _, m := range c.Messages[1:] {
			_, err := repo.NewMessage(ctx, id, m.SenderUserID, m.Text)
			if err != nil {
				panic(err)
			}
		}
	}

	return repo
}
